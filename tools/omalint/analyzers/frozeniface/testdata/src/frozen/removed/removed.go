package connector

type Sink interface { // want "frozeniface: Sink methods"
	AccountStatus()
	Contact()
	Conversation()
	Incoming()
	OutgoingStatus()
}

type Connector interface {
	Account()
	Run()
	Send()
	MarkRead()
}
