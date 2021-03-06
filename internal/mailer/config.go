package mailer

// Config struct encapsulate all settings for TextMailer
type Config struct {
	Server   string
	Port     int
	User     string
	Password string

	From    string
	To      string
	Subject string
}
