package connector

var unrelated int

const unrelatedConstant = 1

type Sink interface {
	AccountStatus()
	Contact()
	Conversation()
	Incoming()
	OutgoingStatus()
	Typing()
}

type Connector interface {
	Account()
	Run()
	Send()
	MarkRead()
}
