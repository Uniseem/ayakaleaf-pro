package settings

// The settings an administrator can change, as data.
//
// The admin page renders whatever is here and knows nothing about any
// particular setting: adding one is adding a row. The page it replaces had the
// same idea, and this keeps it, because it is the only thing that stopped that
// page from growing a special case per field.
//
// A setting with no Path reaches whatever it configures through the process
// environment, and so takes effect when the site restarts. One with a Path is
// written into the running configuration and takes effect at once.

// Kind is how a value is edited and stored.
type Kind string

const (
	KindString   Kind = "string"
	KindText     Kind = "text"
	KindBool     Kind = "boolean"
	KindNumber   Kind = "number"
	KindPassword Kind = "password"
	KindSelect   Kind = "select"
	KindJSON     Kind = "json"
)

// Option is one choice in a select.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Section groups settings on the page.
type Section struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
}

// Definition is one setting.
type Definition struct {
	Key     string   `json:"key"`
	Section string   `json:"section"`
	Kind    Kind     `json:"kind"`
	Label   string   `json:"label"`
	Help    string   `json:"help,omitempty"`
	Options []Option `json:"options,omitempty"`
	// Env is the variable this reaches its reader through, when it does.
	Env string `json:"env,omitempty"`
	// Restart says the value is only read when the site starts.
	Restart bool `json:"restart,omitempty"`
	// Secret is never sent back to a browser, only whether it is set.
	Secret bool `json:"secret,omitempty"`
	// Default is what the setting is when nobody has chosen.
	Default any `json:"default,omitempty"`
}

// Sections in the order the page shows them.
var Sections = []Section{
	{ID: "site", Label: "Site", Help: "What this site is called and where it lives."},
	{ID: "registration", Label: "Sign up and sign in", Help: "Who may create an account."},
	{ID: "oauth", Label: "Google and GitHub", Help: "Leave a client id blank to hide that button."},
	{ID: "email", Label: "Email", Help: "Used for invitations, password resets and notifications."},
	{ID: "compile", Label: "Compiling", Help: "Limits applied to every compile."},
	{ID: "git", Label: "Git and GitHub", Help: "Cloning a project over git, and keeping one in step with a repository."},
	{ID: "security", Label: "Security", Help: "Session and password rules."},
	{ID: "advanced", Label: "Advanced", Help: "Anything without a field of its own."},
}

// Settings in the order the page shows them.
var Settings = []Definition{
	{
		Key: "siteUrl", Section: "site", Kind: KindString, Label: "Site URL",
		Help:    "The address people reach this site at. Links in email and the sign-in callbacks are built from it, so it has to be right.",
		Default: "http://localhost",
	},
	{
		Key: "appName", Section: "site", Kind: KindString, Label: "Site name",
		Default: "Ayakaleaf Pro",
	},
	{
		Key: "adminEmail", Section: "site", Kind: KindString, Label: "Administrator address",
		Help: "Shown to people as the address to ask for help.",
	},

	{
		Key: "allowPublicRegistration", Section: "registration", Kind: KindBool,
		Label:   "Anybody may sign up",
		Help:    "Off means only an administrator can make accounts. The first account is allowed either way, so a new site can be claimed.",
		Default: true,
	},
	{
		Key: "registrationEmailDomains", Section: "registration", Kind: KindString,
		Label: "Restrict sign up to these email domains",
		Help:  "Comma separated, for example example.com, *.example.org. Blank allows any address.",
	},
	{
		Key: "emailConfirmation", Section: "registration", Kind: KindSelect,
		Label: "Confirming an email address",
		Help:  "A site with no mail server cannot ask anybody to confirm anything, so the usual answer follows whether SMTP is configured.",
		Options: []Option{
			{Value: "auto", Label: "required once a mail server is configured"},
			{Value: "required", Label: "always required"},
			{Value: "off", Label: "never asked for"},
		},
		Default: "auto",
	},

	{Key: "googleClientId", Section: "oauth", Kind: KindString, Label: "Google client ID"},
	{Key: "googleClientSecret", Section: "oauth", Kind: KindPassword, Secret: true, Label: "Google client secret"},
	{Key: "githubClientId", Section: "oauth", Kind: KindString, Label: "GitHub client ID"},
	{Key: "githubClientSecret", Section: "oauth", Kind: KindPassword, Secret: true, Label: "GitHub client secret"},
	{
		Key: "oauthCreatesAccounts", Section: "oauth", Kind: KindBool,
		Label:   "Signing in with Google or GitHub may create an account",
		Default: true,
	},

	{Key: "emailFromAddress", Section: "email", Kind: KindString, Label: "From address", Env: "OVERLEAF_EMAIL_FROM_ADDRESS", Restart: true},
	{Key: "emailSmtpHost", Section: "email", Kind: KindString, Label: "SMTP host", Env: "OVERLEAF_EMAIL_SMTP_HOST", Restart: true},
	{Key: "emailSmtpPort", Section: "email", Kind: KindNumber, Label: "SMTP port", Env: "OVERLEAF_EMAIL_SMTP_PORT", Restart: true, Default: 587},
	{Key: "emailSmtpUser", Section: "email", Kind: KindString, Label: "SMTP user", Env: "OVERLEAF_EMAIL_SMTP_USER", Restart: true},
	{Key: "emailSmtpPass", Section: "email", Kind: KindPassword, Secret: true, Label: "SMTP password", Env: "OVERLEAF_EMAIL_SMTP_PASS", Restart: true},
	{Key: "emailSmtpSecure", Section: "email", Kind: KindBool, Label: "Connect with TLS", Env: "OVERLEAF_EMAIL_SMTP_SECURE", Restart: true},

	{
		Key: "compileTimeout", Section: "compile", Kind: KindNumber,
		Label:   "Compile timeout, seconds",
		Help:    "One of the features an account is given when it is created, so changing it applies to accounts made afterwards.",
		Default: 180,
	},
	{
		Key: "sandboxedCompiles", Section: "compile", Kind: KindBool,
		Label:   "Compile each project in its own container",
		Help:    "Without this every compile shares one container, so a project can read another project. The deployment also has to mount the docker socket.",
		Env:     "SANDBOXED_COMPILES",
		Restart: true,
	},
	{
		Key: "texLiveImages", Section: "compile", Kind: KindString,
		Label:   "TeX Live images this site offers",
		Help:    "Comma separated, full references. Pull each one on the host first; nothing here pulls them.",
		Env:     "ALL_TEX_LIVE_DOCKER_IMAGES",
		Restart: true,
	},

	{
		Key: "gitBridgeEnabled", Section: "git", Kind: KindBool,
		Label: "Allow projects to be cloned and pushed over git",
		Help: "Needs the git container as well: start the deployment with " +
			"--profile git. Without it, a clone hangs instead of being refused.",
		Env:     "GIT_BRIDGE_ENABLED",
		Restart: true,
	},
	{
		Key: "gitBridgeHost", Section: "git", Kind: KindString,
		Label:   "Where the git container is",
		Help:    "The name it has on the deployment's network. Leave it alone unless you renamed it.",
		Env:     "GIT_BRIDGE_HOST",
		Default: "git-bridge",
		Restart: true,
	},
	{
		Key: "gitBridgePort", Section: "git", Kind: KindNumber,
		Label:   "The port it listens on",
		Env:     "GIT_BRIDGE_PORT",
		Default: 8000,
		Restart: true,
	},

	{
		Key: "githubSyncEnabled", Section: "git", Kind: KindBool,
		Label: "Keep projects in step with GitHub repositories",
		Help: "Needs a GitHub OAuth application of its own, below. Not the same " +
			"one as GitHub sign-in: this one asks for access to repositories.",
	},
	{
		Key: "githubSyncClientId", Section: "git", Kind: KindString,
		Label: "GitHub sync client ID",
		Help: "Register an OAuth application at github.com/settings/developers " +
			"with the callback URL <site URL>/api/github/callback.",
	},
	{
		Key: "githubSyncClientSecret", Section: "git", Kind: KindPassword, Secret: true,
		Label: "GitHub sync client secret",
	},

	{
		Key: "passwordMinLength", Section: "security", Kind: KindNumber,
		Label: "Shortest password", Default: 8,
	},
	{
		Key: "passwordMaxLength", Section: "security", Kind: KindNumber,
		Label:   "Longest password",
		Help:    "bcrypt stops reading at 72 bytes, so a larger number would be a promise the hash cannot keep.",
		Default: 72,
	},
	{
		Key: "cookieSessionLength", Section: "security", Kind: KindNumber,
		Label:   "How long a session lasts, in milliseconds",
		Help:    "Five days by default. Milliseconds, not seconds: read as seconds it would sign everybody out after seven minutes.",
		Default: 432000000,
		Restart: true,
	},
	{
		Key: "secureCookie", Section: "security", Kind: KindBool,
		Label:   "Only send the session cookie over HTTPS",
		Restart: true,
	},

	{
		Key: "extraEnvironment", Section: "advanced", Kind: KindText,
		Label:   "Anything else, as NAME=value lines",
		Help:    "One per line, put into the environment before anything reads it. Unlike the password fields above, what you type here is shown back to anybody who can open this page.",
		Restart: true,
	},
}

// definitionsByKey is how a save finds out what it is saving.
var definitionsByKey = func() map[string]Definition {
	byKey := make(map[string]Definition, len(Settings))
	for _, definition := range Settings {
		byKey[definition.Key] = definition
	}
	return byKey
}()

// DefinitionFor returns a setting's definition.
func DefinitionFor(key string) (Definition, bool) {
	definition, ok := definitionsByKey[key]
	return definition, ok
}
