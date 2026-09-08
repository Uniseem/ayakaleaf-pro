package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Settings that are delivered as environment variables.
//
// Most of what the other services read is read while their settings file is
// being evaluated, before any of them could ask a database anything. So the
// stored settings are written into the directory the container builds every
// service's environment from, before any service starts. That is what makes
// those settings say they need a restart: this is the moment they take effect.
//
// The table below is longer than the admin pages are. Every variable an
// operator could have set before the rewrite is here, so that upgrading does
// not silently unset one; the pages offer the ones this version can honour.

type envKind int

const (
	kindString envKind = iota
	kindBool
	kindNumber
)

type envSetting struct {
	key      string
	env      string
	kind     envKind
	fallback string
}

// environmentSettings is the whole table, generated from what the settings
// page offered before the rewrite so that nothing an operator set is lost.
var environmentSettings = []envSetting{
	{key: "siteUrl", env: "OVERLEAF_SITE_URL", kind: kindString, fallback: "http://localhost"},
	{key: "appName", env: "OVERLEAF_APP_NAME", kind: kindString, fallback: "Ayakaleaf Pro"},
	{key: "adminEmail", env: "OVERLEAF_ADMIN_EMAIL", kind: kindString},
	{key: "siteLanguage", env: "OVERLEAF_SITE_LANGUAGE", kind: kindString, fallback: "en"},
	{key: "statusPageUrl", env: "OVERLEAF_STATUS_PAGE_URL", kind: kindString},
	{key: "allowPublicRegistration", env: "OVERLEAF_ALLOW_PUBLIC_REGISTRATION", kind: kindBool, fallback: "true"},
	{key: "allowAnonymousReadAndWriteSharing", env: "OVERLEAF_ALLOW_ANONYMOUS_READ_AND_WRITE_SHARING", kind: kindBool, fallback: "false"},
	{key: "restrictInvitesToExistingAccounts", env: "OVERLEAF_RESTRICT_INVITES_TO_EXISTING_ACCOUNTS", kind: kindBool, fallback: "false"},
	{key: "emailFromAddress", env: "OVERLEAF_EMAIL_FROM_ADDRESS", kind: kindString},
	{key: "emailReplyTo", env: "OVERLEAF_EMAIL_REPLY_TO", kind: kindString},
	{key: "emailSmtpHost", env: "OVERLEAF_EMAIL_SMTP_HOST", kind: kindString},
	{key: "emailSmtpPort", env: "OVERLEAF_EMAIL_SMTP_PORT", kind: kindNumber, fallback: "587"},
	{key: "emailSmtpSecure", env: "OVERLEAF_EMAIL_SMTP_SECURE", kind: kindBool, fallback: "false"},
	{key: "emailSmtpUser", env: "OVERLEAF_EMAIL_SMTP_USER", kind: kindString},
	{key: "emailSmtpPass", env: "OVERLEAF_EMAIL_SMTP_PASS", kind: kindString},
	{key: "emailSmtpIgnoreTLS", env: "OVERLEAF_EMAIL_SMTP_IGNORE_TLS", kind: kindBool, fallback: "false"},
	{key: "emailCustomFooter", env: "OVERLEAF_CUSTOM_EMAIL_FOOTER", kind: kindString},
	{key: "navTitle", env: "OVERLEAF_NAV_TITLE", kind: kindString},
	{key: "headerImageUrl", env: "OVERLEAF_HEADER_IMAGE_URL", kind: kindString},
	{key: "leftFooter", env: "OVERLEAF_LEFT_FOOTER", kind: kindString},
	{key: "rightFooter", env: "OVERLEAF_RIGHT_FOOTER", kind: kindString},
	{key: "maintenanceMessage", env: "OVERLEAF_MAINTENANCE_MESSAGE", kind: kindString},
	{key: "compileTimeout", env: "COMPILE_TIMEOUT", kind: kindNumber, fallback: "180"},
	{key: "secureCookie", env: "OVERLEAF_SECURE_COOKIE", kind: kindBool, fallback: "false"},
	{key: "cookieSessionLength", env: "OVERLEAF_COOKIE_SESSION_LENGTH", kind: kindNumber, fallback: "432000000"},
	{key: "allowPublicAccess", env: "OVERLEAF_ALLOW_PUBLIC_ACCESS", kind: kindBool, fallback: "false"},
	{key: "disableLinkSharing", env: "OVERLEAF_DISABLE_LINK_SHARING", kind: kindBool, fallback: "false"},
	{key: "disableChat", env: "OVERLEAF_DISABLE_CHAT", kind: kindBool, fallback: "false"},
	{key: "loginSupportText", env: "OVERLEAF_LOGIN_SUPPORT_TEXT", kind: kindString},
	{key: "defaultCompiler", env: "DEFAULT_LATEX_COMPILER", kind: kindString, fallback: "pdflatex"},
	{key: "maxUploadSize", env: "MAX_UPLOAD_SIZE", kind: kindNumber, fallback: "50"},
	{key: "compileSizeLimit", env: "COMPILE_SIZE_LIMIT", kind: kindString, fallback: "7mb"},
	{key: "additionalTextExtensions", env: "ADDITIONAL_TEXT_EXTENSIONS", kind: kindString},
	{key: "enabledLinkedFileTypes", env: "ENABLED_LINKED_FILE_TYPES", kind: kindString, fallback: "project_file,project_output_file"},
	{key: "maintenanceMessageHtml", env: "OVERLEAF_MAINTENANCE_MESSAGE_HTML", kind: kindString},
	{key: "cronResourceDeletion", env: "ENABLE_CRON_RESOURCE_DELETION", kind: kindBool, fallback: "false"},
	{key: "userDeletionDelay", env: "OVERLEAF_USER_HARD_DELETION_DELAY", kind: kindNumber, fallback: "7776000000"},
	{key: "projectDeletionDelay", env: "OVERLEAF_PROJECT_HARD_DELETION_DELAY", kind: kindNumber, fallback: "7776000000"},
	{key: "gitBridgeEnabled", env: "GIT_BRIDGE_ENABLED", kind: kindBool, fallback: "false"},
	{key: "gitBridgeHost", env: "GIT_BRIDGE_HOST", kind: kindString, fallback: "git-bridge"},
	{key: "gitBridgePort", env: "GIT_BRIDGE_PORT", kind: kindNumber, fallback: "8000"},
	{key: "githubSyncEnabled", env: "GITHUB_SYNC_ENABLED", kind: kindBool, fallback: "false"},
	{key: "githubSyncClientId", env: "GITHUB_SYNC_CLIENT_ID", kind: kindString},
	{key: "githubSyncClientSecret", env: "GITHUB_SYNC_CLIENT_SECRET", kind: kindString},
	{key: "githubTokenCipherPassword", env: "GITHUB_TOKEN_CIPHER_PASSWORD", kind: kindString},
	{key: "githubSyncProxyUrl", env: "GITHUB_SYNC_PROXY_URL", kind: kindString},
	{key: "zoteroClientId", env: "ZOTERO_CLIENT_ID", kind: kindString},
	{key: "zoteroClientSecret", env: "ZOTERO_CLIENT_SECRET", kind: kindString},
	{key: "zoteroProxyUrl", env: "ZOTERO_PROXY_URL", kind: kindString},
	{key: "mendeleyClientId", env: "MENDELEY_CLIENT_ID", kind: kindString},
	{key: "mendeleyClientSecret", env: "MENDELEY_CLIENT_SECRET", kind: kindString},
	{key: "mendeleyProxyUrl", env: "MENDELEY_PROXY_URL", kind: kindString},
	{key: "templateGallery", env: "OVERLEAF_TEMPLATE_GALLERY", kind: kindBool, fallback: "false"},
	{key: "enableConversions", env: "ENABLE_CONVERSIONS", kind: kindBool, fallback: "false"},
	{key: "templateCategories", env: "OVERLEAF_TEMPLATE_CATEGORIES", kind: kindString},
	{key: "templatesUserId", env: "OVERLEAF_TEMPLATES_USER_ID", kind: kindString},
	{key: "sandboxedCompiles", env: "SANDBOXED_COMPILES", kind: kindBool, fallback: "false"},
	{key: "imageRoot", env: "IMAGE_ROOT", kind: kindString, fallback: "ghcr.io/ayaka-notes"},
	{key: "texLiveImages", env: "ALL_TEX_LIVE_DOCKER_IMAGES", kind: kindString},
	{key: "texLiveImageNames", env: "ALL_TEX_LIVE_DOCKER_IMAGE_NAMES", kind: kindString},
	{key: "texLiveDefaultImage", env: "TEX_LIVE_DOCKER_IMAGE", kind: kindString},
	{key: "filestoreBackend", env: "OVERLEAF_FILESTORE_BACKEND", kind: kindString, fallback: "fs"},
	{key: "historyGlobalBlobsBucket", env: "OVERLEAF_HISTORY_BLOBS_BUCKET", kind: kindString},
	{key: "historyZipsBucket", env: "OVERLEAF_HISTORY_ZIPS_BUCKET", kind: kindString},
	{key: "filestoreTemplateBucket", env: "OVERLEAF_FILESTORE_TEMPLATE_FILES_BUCKET_NAME", kind: kindString},
	{key: "filestoreS3Key", env: "OVERLEAF_FILESTORE_S3_ACCESS_KEY_ID", kind: kindString},
	{key: "filestoreS3Secret", env: "OVERLEAF_FILESTORE_S3_SECRET_ACCESS_KEY", kind: kindString},
	{key: "filestoreS3Region", env: "OVERLEAF_FILESTORE_S3_REGION", kind: kindString},
	{key: "filestoreS3Endpoint", env: "OVERLEAF_FILESTORE_S3_ENDPOINT", kind: kindString},
	{key: "filestoreS3PathStyle", env: "OVERLEAF_FILESTORE_S3_PATH_STYLE", kind: kindBool, fallback: "false"},
	{key: "historyBackend", env: "OVERLEAF_HISTORY_BACKEND", kind: kindString, fallback: "fs"},
	{key: "historyBlobsBucket", env: "OVERLEAF_HISTORY_PROJECT_BLOBS_BUCKET", kind: kindString},
	{key: "historyChunksBucket", env: "OVERLEAF_HISTORY_CHUNKS_BUCKET", kind: kindString},
	{key: "historyS3Key", env: "OVERLEAF_HISTORY_S3_ACCESS_KEY_ID", kind: kindString},
	{key: "historyS3Secret", env: "OVERLEAF_HISTORY_S3_SECRET_ACCESS_KEY", kind: kindString},
	{key: "historyS3Region", env: "OVERLEAF_HISTORY_S3_REGION", kind: kindString},
	{key: "historyS3Endpoint", env: "OVERLEAF_HISTORY_S3_ENDPOINT", kind: kindString},
	{key: "historyS3PathStyle", env: "OVERLEAF_HISTORY_S3_PATH_STYLE", kind: kindBool, fallback: "false"},
	{key: "externalAuth", env: "EXTERNAL_AUTH", kind: kindString},
	{key: "ldapUrl", env: "OVERLEAF_LDAP_URL", kind: kindString},
	{key: "ldapSearchBase", env: "OVERLEAF_LDAP_SEARCH_BASE", kind: kindString},
	{key: "ldapSearchFilter", env: "OVERLEAF_LDAP_SEARCH_FILTER", kind: kindString},
	{key: "ldapBindDn", env: "OVERLEAF_LDAP_BIND_DN", kind: kindString},
	{key: "ldapBindCredentials", env: "OVERLEAF_LDAP_BIND_CREDENTIALS", kind: kindString},
	{key: "ldapEmailAtt", env: "OVERLEAF_LDAP_EMAIL_ATT", kind: kindString, fallback: "mail"},
	{key: "ldapNameAtt", env: "OVERLEAF_LDAP_NAME_ATT", kind: kindString},
	{key: "ldapLastNameAtt", env: "OVERLEAF_LDAP_LAST_NAME_ATT", kind: kindString},
	{key: "ldapIdentityServiceName", env: "OVERLEAF_LDAP_IDENTITY_SERVICE_NAME", kind: kindString},
	{key: "ldapUpdateOnLogin", env: "OVERLEAF_LDAP_UPDATE_USER_DETAILS_ON_LOGIN", kind: kindBool, fallback: "false"},
	{key: "samlEntrypoint", env: "OVERLEAF_SAML_ENTRYPOINT", kind: kindString},
	{key: "samlIssuer", env: "OVERLEAF_SAML_ISSUER", kind: kindString},
	{key: "samlIdpCert", env: "OVERLEAF_SAML_IDP_CERT", kind: kindString},
	{key: "samlEmailField", env: "OVERLEAF_SAML_EMAIL_FIELD", kind: kindString},
	{key: "samlFirstNameField", env: "OVERLEAF_SAML_FIRST_NAME_FIELD", kind: kindString},
	{key: "samlLastNameField", env: "OVERLEAF_SAML_LAST_NAME_FIELD", kind: kindString},
	{key: "samlIdentityServiceName", env: "OVERLEAF_SAML_IDENTITY_SERVICE_NAME", kind: kindString},
	{key: "oidcIssuer", env: "OVERLEAF_OIDC_ISSUER", kind: kindString},
	{key: "oidcClientId", env: "OVERLEAF_OIDC_CLIENT_ID", kind: kindString},
	{key: "oidcClientSecret", env: "OVERLEAF_OIDC_CLIENT_SECRET", kind: kindString},
	{key: "oidcAuthorizationUrl", env: "OVERLEAF_OIDC_AUTHORIZATION_URL", kind: kindString},
	{key: "oidcTokenUrl", env: "OVERLEAF_OIDC_TOKEN_URL", kind: kindString},
	{key: "oidcUserInfoUrl", env: "OVERLEAF_OIDC_USER_INFO_URL", kind: kindString},
	{key: "oidcScope", env: "OVERLEAF_OIDC_SCOPE", kind: kindString, fallback: "openid profile email"},
	{key: "oidcIdentityServiceName", env: "OVERLEAF_OIDC_IDENTITY_SERVICE_NAME", kind: kindString},
	{key: "oidcAllowedEmailDomains", env: "OVERLEAF_OIDC_ALLOWED_EMAIL_DOMAINS", kind: kindString},
	{key: "implChat", env: "CHAT_IMPL", kind: kindString},
	{key: "implDocstore", env: "DOCSTORE_IMPL", kind: kindString},
	{key: "implDocumentUpdater", env: "DOCUPDATER_IMPL", kind: kindString},
	{key: "implFilestore", env: "FILESTORE_IMPL", kind: kindString},
	{key: "implLinkedUrlProxy", env: "LINKED_URL_PROXY_IMPL", kind: kindString},
	{key: "implNotifications", env: "NOTIFICATIONS_IMPL", kind: kindString},
	{key: "implProjectHistory", env: "PROJECTHISTORY_IMPL", kind: kindString},
	{key: "implRealTime", env: "REALTIME_IMPL", kind: kindString},
}

// Environment is every variable these settings amount to.
//
// A variable is absent when it should not be set at all: unset and false are
// the same thing to most of what reads these, and several of them treat the
// presence of the variable as the answer.
func Environment(values map[string]json.RawMessage) map[string]string {
	environment := map[string]string{}

	for _, setting := range environmentSettings {
		text, ok := environmentValue(setting, values[setting.key])
		if !ok {
			continue
		}
		environment[setting.env] = text
	}

	// A reference manager is reachable only if its kind of linked file is
	// allowed, so configuring one implies allowing it. Asking an administrator
	// to remember a second list as well is how one ends up configured and
	// invisible.
	linked := map[string]bool{}
	for _, entry := range strings.Split(environment["ENABLED_LINKED_FILE_TYPES"], ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			linked[entry] = true
		}
	}
	if stringValue(values["zoteroClientId"]) != "" {
		linked["zotero"] = true
	}
	if stringValue(values["mendeleyClientId"]) != "" {
		linked["mendeley"] = true
	}
	if len(linked) > 0 {
		names := make([]string, 0, len(linked))
		for entry := range linked {
			names = append(names, entry)
		}
		sort.Strings(names)
		environment["ENABLED_LINKED_FILE_TYPES"] = strings.Join(names, ",")
	}

	// Last, so a name set here wins: this is the escape hatch, and it is no
	// use if a field it does not know about overrides it.
	for entry, value := range extraEnvironment(stringValue(values["extraEnvironment"])) {
		environment[entry] = value
	}
	return environment
}

// WriteEnvironment puts the settings into the directory the container builds
// every service's environment from, and answers with how many it wrote.
func WriteEnvironment(directory, manifestPath string, values map[string]json.RawMessage) (int, error) {
	environment := Environment(values)

	// Anything written last time and not now is a setting that has been turned
	// off. Only those are removed: a variable set in the compose file and never
	// here is not ours to take away.
	if previous, err := os.ReadFile(manifestPath); err == nil {
		var written []string
		if json.Unmarshal(previous, &written) == nil {
			for _, entry := range written {
				if _, keeping := environment[entry]; !keeping {
					_ = os.Remove(filepath.Join(directory, entry))
				}
			}
		}
	}

	names := make([]string, 0, len(environment))
	for entry, value := range environment {
		path := filepath.Join(directory, entry)
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			return 0, fmt.Errorf("could not write %s: %w", entry, err)
		}
		names = append(names, entry)
	}
	sort.Strings(names)

	manifest, err := json.Marshal(names)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		return 0, err
	}
	return len(names), nil
}

// environmentValue turns a stored value into the text a variable carries, and
// says whether the variable should exist at all.
func environmentValue(setting envSetting, raw json.RawMessage) (string, bool) {
	switch setting.kind {
	case kindBool:
		value, present := boolValue(raw)
		if !present {
			value = setting.fallback == "true"
		}
		if !value {
			return "", false
		}
		return "true", true

	case kindNumber:
		text := numberValue(raw)
		if text == "" {
			text = setting.fallback
		}
		if text == "" {
			return "", false
		}
		return text, true

	default:
		text := stringValue(raw)
		if text == "" {
			text = setting.fallback
		}
		if text == "" {
			return "", false
		}
		return text, true
	}
}

func stringValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	var flag bool
	if json.Unmarshal(raw, &flag) == nil {
		return strconv.FormatBool(flag)
	}
	return ""
}

func boolValue(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var flag bool
	if json.Unmarshal(raw, &flag) == nil {
		return flag, true
	}
	// Stored as the text a form sent, which is what a checkbox that went
	// through a query string leaves behind.
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.EqualFold(strings.TrimSpace(text), "true"), true
	}
	return false, false
}

func numberValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if _, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

// variableName is what an environment variable may be called. Anything else in
// the free-text block is skipped rather than guessed at: a typo should lose one
// variable, not the rest of the block.
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// extraEnvironment reads NAME=value lines.
func extraEnvironment(text string) map[string]string {
	extra := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		at := strings.IndexByte(trimmed, '=')
		if at <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:at])
		if !variableName.MatchString(key) {
			continue
		}
		extra[key] = strings.TrimSpace(trimmed[at+1:])
	}
	return extra
}
