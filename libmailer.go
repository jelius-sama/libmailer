package main

/*
 */
import "C"

import (
	"github.com/jelius-sama/libmailer/api"
)

func LoadConfig() (*api.Config, error) {
	return api.LoadConfig()
}

func LoadConfigFromPath(configPath string) (*api.Config, error) {
	return api.LoadConfigFromPath(configPath)
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
