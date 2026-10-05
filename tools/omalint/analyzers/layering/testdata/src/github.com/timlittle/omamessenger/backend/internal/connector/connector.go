package connector

import (
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store" // want "layering:"
	//omalint:ignore layering fixture isolates a deliberately forbidden import
	"github.com/timlittle/omamessenger/backend/internal/notify"
	//omalint:ignore layering // want "suppression requires"
	"github.com/timlittle/omamessenger/backend/internal/app/policy" // want "layering:"
)

var _ domain.Message
var _ store.Store
var _ notify.Notifier
var _ policy.Policy
