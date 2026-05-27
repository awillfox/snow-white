package profile

// Profile holds saved connection details.
// SSLMode is "require" or "disable" (maps to the SSL toggle in the TUI).
// Password is stored in plaintext — users are warned before saving.
type Profile struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"ssl_mode"`
}
