package persistor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config mirrors the settings.filestore.s3 block.
type S3Config struct {
	Key       string
	Secret    string
	Endpoint  string
	Region    string
	PathStyle bool
	// PartSize is the multipart upload chunk size; the Node setting defaults
	// to 100 MB.
	PartSize int64
	// SignedURLExpiry bounds a redirect URL's lifetime.
	SignedURLExpiry time.Duration
}

// S3 stores objects in S3 or an S3-compatible service such as MinIO.
type S3 struct {
	client  *s3.Client
	presign *s3.PresignClient
	cfg     S3Config
}

// NewS3 builds an S3 persistor.
//
// Credentials come from the configuration when given and otherwise from the
// usual AWS environment, so an instance role works without extra settings.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if cfg.PartSize <= 0 {
		cfg.PartSize = 100 * 1024 * 1024
	}
	if cfg.SignedURLExpiry <= 0 {
		cfg.SignedURLExpiry = time.Minute
	}
	region := cfg.Region
	if region == "" {
		// An S3-compatible endpoint usually ignores the region, but the SDK
		// insists on one being set.
		region = "us-east-1"
	}

	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.Key != "" && cfg.Secret != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.Key, cfg.Secret, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// MinIO and most other S3-compatible services need path-style
		// addressing; virtual-host style would resolve the bucket as a
		// subdomain.
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3{client: client, presign: s3.NewPresignClient(client), cfg: cfg}, nil
}

// notFound reports whether an S3 error means the object or bucket is absent.
func notFound(err error) bool {
	var nsk *types.NoSuchKey
	var nsb *types.NoSuchBucket
	var nf *types.NotFound
	return errors.As(err, &nsk) || errors.As(err, &nsb) || errors.As(err, &nf)
}

func (p *S3) GetObject(ctx context.Context, bucket, key string, opts GetOptions) (io.ReadCloser, error) {
	in := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
	if opts.HasRange {
		in.Range = aws.String(fmt.Sprintf("bytes=%d-%d", opts.Start, opts.End))
	}
	out, err := p.client.GetObject(ctx, in)
	if notFound(err) {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, bucket, key)
	}
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (p *S3) GetObjectSize(ctx context.Context, bucket, key string, opts GetOptions) (int64, error) {
	out, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	})
	if notFound(err) {
		return 0, fmt.Errorf("%w: %s/%s", ErrNotFound, bucket, key)
	}
	if err != nil {
		return 0, err
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

func (p *S3) SendStream(ctx context.Context, bucket, key string, r io.Reader, opts GetOptions) error {
	uploader := manager.NewUploader(p.client, func(u *manager.Uploader) {
		u.PartSize = p.cfg.PartSize
	})
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: r,
	})
	return err
}

func (p *S3) SendFile(ctx context.Context, bucket, key, fsPath string, opts GetOptions) error {
	f, err := os.Open(fsPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return p.SendStream(ctx, bucket, key, f, opts)
}

func (p *S3) DeleteObject(ctx context.Context, bucket, key string, opts GetOptions) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	})
	if notFound(err) {
		// Deleting something absent is not an error.
		return nil
	}
	return err
}

func (p *S3) DeleteDirectory(ctx context.Context, bucket, prefix string, opts GetOptions) error {
	// A prefix names a pseudo-directory, so it must end in a separator or
	// "abc" would also match "abcd".
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	paginator := s3.NewListObjectsV2Paginator(p.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		if _, err := p.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *S3) ObjectExists(ctx context.Context, bucket, key string, opts GetOptions) (bool, error) {
	_, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	})
	if notFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// RedirectURL returns a presigned GET the client can follow directly, which
// keeps large downloads off this service entirely.
func (p *S3) RedirectURL(ctx context.Context, bucket, key string) (string, error) {
	req, err := p.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(p.cfg.SignedURLExpiry))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
