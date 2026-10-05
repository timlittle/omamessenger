package domain

type Account struct{ Name string }
type Contact struct{ Name string }
type Conversation struct{ Title, Preview, PreviewSender, Match string }
type Message struct{ Text, SenderName string }
