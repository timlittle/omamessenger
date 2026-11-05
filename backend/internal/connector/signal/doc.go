// Package signal connects a Signal account through signalmeow: it links
// as a secondary device by QR code, imports the history Signal transfers
// at link time, follows live messages, receipts, typing, reactions,
// edits, deletes, polls and disappearing timers, and normalizes
// signalmeow's events into the domain types the rest of the helper uses.
// Sending and acting on messages (replying, reacting, deleting) are a
// later addition; this package only links and receives.
package signal
