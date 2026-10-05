package connector

type Sink interface { // want "frozeniface: Sink methods"
	AccountStatus()
	Contact()
	Conversation()
	Incoming()
	OutgoingStatus()
	Typing()
	NewMethod()
}

//omalint:ignore frozeniface fixture keeps a local experiment quiet
type Connector interface {
	Account()
	Run()
	Send()
	MarkRead()
	Extra()
}
