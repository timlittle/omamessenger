package store

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

type unrelated struct{ Name string }

func allowed(m domain.Message, local unrelated) {
	fmt.Print("message received")
	fmt.Fprintln(os.Stderr, "message received")
	fmt.Print(local.Name)
	_ = fmt.Errorf("safe formatting %v", m.Text)
	log.New(io.Discard, "", 0).Print("discarded")
	_ = m
}

func fullValue(m domain.Message) {
	log.Print(m)                                 // want "nologcontent: sensitive messaging data"
	log.New(io.Discard, "", 0).Print(m)          // want "nologcontent: sensitive messaging data"
	log.Print(fmt.Sprintf("message %s", m.Text)) // want "nologcontent: sensitive messaging data"
}

func containers(m domain.Message) {
	log.Print([]domain.Message{m})                 // want "nologcontent: sensitive messaging data"
	log.Print([1]domain.Message{m})                // want "nologcontent: sensitive messaging data"
	log.Print(map[string]domain.Message{"key": m}) // want "nologcontent: sensitive messaging data"
}

func contentFields(m domain.Message, c domain.Conversation, a domain.Account) {
	slog.Info("message", "body", m.Text) // want "nologcontent: sensitive messaging data"
	fmt.Fprintln(os.Stdout, c.Preview)   // want "nologcontent: sensitive messaging data"
	fmt.Printf("account %s", a.Name)     // want "nologcontent: sensitive messaging data"
	fmt.Print(c.PreviewSender)           // want "nologcontent: sensitive messaging data"
	fmt.Print(c.Match)                   // want "nologcontent: sensitive messaging data"
	fmt.Print(m.SenderName)              // want "nologcontent: sensitive messaging data"
	fmt.Print(domain.Contact{Name: "x"}) // want "nologcontent: sensitive messaging data"
}

func pointersAndChannels(m *domain.Message, ch chan domain.Message) {
	log.Print(m)  // want "nologcontent: sensitive messaging data"
	log.Print(ch) // want "nologcontent: sensitive messaging data"
}

// There is no suppression: a comment does not silence the privacy rule.
func noSuppression(m domain.Message) {
	//nolint:nologcontent // tried anyway
	log.Print(m) // want "nologcontent: sensitive messaging data"
}

func noFprintToFile(m domain.Message, file *os.File) {
	fmt.Fprint(file, m.Text)
}

func noSensitiveReceiver(m domain.Message) {
	var value struct{ Text string }
	fmt.Print(value.Text)
	_ = m
}
