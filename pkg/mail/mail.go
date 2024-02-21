package mail

import (
	"github.com/mylxsw/aidea-chat-server/config"
	"gopkg.in/gomail.v2"
)

type Sender struct {
	conf   config.Mail
	dialer *gomail.Dialer
}

func NewSender(conf config.Mail) *Sender {
	dialer := gomail.NewDialer(conf.Host, conf.Port, conf.Username, conf.Password)
	dialer.SSL = conf.UseSSL

	return &Sender{conf: conf, dialer: dialer}
}

func (m *Sender) Send(to []string, subject, body string) error {
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", m.conf.Username, m.conf.From)
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", body)

	return m.dialer.DialAndSend(msg)
}
