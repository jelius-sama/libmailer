package main

/*
typedef struct {
	char *Host;
	int Port;
	char *Username;
	char *Password;
	char *From;
} MailerConfig;
*/
import "C"

import (
	"github.com/jelius-sama/libmailer/api"
)

//export LoadConfig
func LoadConfig() (*C.MailerConfig, error) {
	var mailerConfig C.MailerConfig
	cnf, err := api.LoadConfig()

	if err != nil {
		return nil, err
	}

	mailerConfig.Host = C.CString(cnf.Host)
	mailerConfig.Port = C.int(cnf.Port)
	mailerConfig.Username = C.CString(cnf.Username)
	mailerConfig.Password = C.CString(cnf.Password)
	mailerConfig.From = C.CString(cnf.From)
	return &mailerConfig, err
}

//export LoadConfigFromPath
func LoadConfigFromPath(configPath string) (*C.MailerConfig, error) {
	var mailerConfig C.MailerConfig
	cnf, err := api.LoadConfigFromPath(configPath)

	if err != nil {
		return nil, err
	}

	mailerConfig.Host = C.CString(cnf.Host)
	mailerConfig.Port = C.int(cnf.Port)
	mailerConfig.Username = C.CString(cnf.Username)
	mailerConfig.Password = C.CString(cnf.Password)
	mailerConfig.From = C.CString(cnf.From)
	return &mailerConfig, err
}

//export ParseEmailAddress
func ParseEmailAddress(addr string) (string, error) {
	return api.ParseEmailAddress(addr)
}

//export FormatEmailAddress
func FormatEmailAddress(addr string) string {
	return api.FormatEmailAddress(addr)
}

//export SendMail
func SendMail(smtpHost string, smtpPort int, username, password, from, to, subject, body string, cc, bcc []string, attachments []string) error {
	return api.SendMail(smtpHost, smtpPort, username, password, from, to, subject, body, cc, bcc, attachments)
}

//export SendRawEML
func SendRawEML(smtpHost string, smtpPort int, username, password string, emlPath string) error {
	return api.SendRawEML(smtpHost, smtpPort, username, password, emlPath)
}

func main() {}
