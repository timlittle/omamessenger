package connector

import (
	"reflect"
	"sort"
	"testing"
)

// TestCoreInterfacesAreFrozen guards the C4 rule: Sink and Connector never
// gain or lose methods. A new capability is a new optional interface, so
// existing connectors and fakes keep compiling. Changing these lists needs a
// C4 update in docs/TASKS.md.
func TestCoreInterfacesAreFrozen(t *testing.T) {
	for _, c := range []struct {
		iface reflect.Type
		want  []string
	}{
		{reflect.TypeFor[Sink](), []string{"AccountStatus", "Contact", "Conversation", "Incoming", "OutgoingStatus", "Typing"}},
		{reflect.TypeFor[Connector](), []string{"Account", "MarkRead", "Run", "Send"}},
	} {
		var got []string
		for i := 0; i < c.iface.NumMethod(); i++ {
			got = append(got, c.iface.Method(i).Name)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s methods = %v, want %v (add an optional interface instead; see C4)", c.iface.Name(), got, c.want)
		}
	}
}
