package db_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tink/client/informers"
	"github.com/tinkerbell/tink/db"
	"github.com/tinkerbell/tink/pkg"
	"github.com/tinkerbell/tink/protos/events"
	"github.com/tinkerbell/tink/protos/hardware"
)

func TestEventsForHardware(t *testing.T) {
	tests := []struct {
		// Name identifies the single test in a table test scenario
		Name string
		// Input is a list of hardwares that will be used to pre-populate the database
		Input []*hardware.Hardware
		// Expectation is the function used to apply the assertions.
		// You can use it to validate if the Input are created as you expect
		Expectation func(*testing.T, []*hardware.Hardware, *db.TinkDB)
		// ExpectedErr is used to check for error during
		// CreateTemplate execution. If you expect a particular error
		// and you want to assert it, you can use this function
		ExpectedErr func(*testing.T, error)
	}{
		{
			Name: "single-hardware-create-event",
			Input: []*hardware.Hardware{
				readHardwareData("./testdata/hardware.json"),
			},
			Expectation: func(t *testing.T, input []*hardware.Hardware, tinkDB *db.TinkDB) {
				err := tinkDB.Events(&events.WatchRequest{}, func(n informers.Notification) error {
					event, err := n.ToEvent()
					if err != nil {
						return err
					}

					if event.EventType != events.EventType_EVENT_TYPE_CREATED {
						return fmt.Errorf("unexpected event type: %s", event.EventType)
					}

					hw, err := getHardwareFromEventData(event)
					if err != nil {
						return err
					}
					if dif := cmp.Diff(input[0], hw, cmp.Comparer(hardwareComparer)); dif != "" {
						t.Errorf(dif)
					}
					return nil
				})
				if err != nil {
					t.Error(err)
				}
			},
		},
	}

	ctx := context.Background()
	for _, s := range tests {
		t.Run(s.Name, func(t *testing.T) {
			_, tinkDB, cl := NewPostgresDatabaseClient(t, ctx, NewPostgresDatabaseRequest{
				ApplyMigration: true,
			})
			defer func() {
				err := cl()
				if err != nil {
					t.Error(err)
				}
			}()
			for _, hw := range s.Input {
				err := createHardware(ctx, tinkDB, hw)
				if err != nil {
					s.ExpectedErr(t, err)
				}
			}
			s.Expectation(t, s.Input, tinkDB)
		})
	}
}

func readHardwareData(file string) *hardware.Hardware {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}
	var hw pkg.HardwareWrapper
	err = json.Unmarshal([]byte(data), &hw)
	if err != nil {
		panic(err)
	}
	return hw.Hardware
}

func createHardware(ctx context.Context, db *db.TinkDB, hw *hardware.Hardware) error {
	data, err := json.Marshal(hw)
	if err != nil {
		return err
	}
	return db.InsertIntoDB(ctx, string(data))
}

func getHardwareFromEventData(event *events.Event) (*hardware.Hardware, error) {
	d, err := base64.StdEncoding.DecodeString(strings.Trim(string(event.Data), "\""))
	if err != nil {
		return nil, err
	}

	hd := &struct {
		Data *hardware.Hardware
	}{}

	err = json.Unmarshal(d, hd)
	if err != nil {
		return nil, err
	}
	return hd.Data, nil
}

func hardwareComparer(in *hardware.Hardware, hw *hardware.Hardware) bool {
	return in.Id == hw.Id &&
		in.Version == hw.Version &&
		strings.EqualFold(in.Metadata, hw.Metadata) &&
		strings.EqualFold(in.Network.Interfaces[0].Dhcp.Mac, hw.Network.Interfaces[0].Dhcp.Mac)
}
