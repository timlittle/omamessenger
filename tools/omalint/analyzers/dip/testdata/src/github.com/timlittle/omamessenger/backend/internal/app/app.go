package app

import (
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

type local struct{}

func NewAllowed(account domain.Account, port connector.Port, name string) {}

func NewBad(db *store.Store) {} // want "dip: constructor NewBad takes concrete internal dependency"

//omalint:ignore dip narrowly scoped suppression fixture
func NewSuppressed(db *store.Store) {}

//omalint:ignore dip // want "suppression requires"
func NewMissingReason(db *store.Store) {} // want "dip: constructor NewMissingReason takes concrete internal dependency"

func NewLocal(value local)               {}
func privateConstructor(db *store.Store) {}
