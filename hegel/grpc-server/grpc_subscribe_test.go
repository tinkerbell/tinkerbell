package grpcserver

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/packethost/cacher/protos/cacher"
	assert "github.com/stretchr/testify/require"
	"github.com/tinkerbell/hegel/grpc/protos/hegel"
	_ "github.com/tinkerbell/hegel/metrics" // Initialize metrics.
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSubscribe(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		terr := status.Error(codes.Unknown, "error pushing")
		ctx, cancel, _, client := startServersAndConnectClient(t, nil, terr)
		defer cancel()

		w, err := client.Subscribe(ctx, &hegel.SubscribeRequest{ID: "doesn't matter"})
		assert.NoError(t, err)
		assert.NotNil(t, w)

		hw, err := w.Recv()
		assertGRPCError(t, err, terr)
		assert.Nil(t, hw)
	})

	t.Run("good", func(t *testing.T) {
		id := "bufconn"
		value := fmt.Sprintf(`{"id": "%s", "ip": "%s"}`, id, id)
		payload := fmt.Sprintf(`%s=%s`, id, value)
		data := map[string]string{
			id: value,
		}

		ctx, cancel, cClient, hClient := startServersAndConnectClient(t, data, nil)
		defer cancel()

		w, err := hClient.Subscribe(ctx, &hegel.SubscribeRequest{ID: id})
		assert.NoError(t, err)
		assert.NotNil(t, w)

		// continually push in the background until w.RecvMsg works
		// otherwise the Push may happen before the watch is setup and thus
		// there is no channel and Push doesn't send anything and RecvMsg
		// hangs
		initialPushes := make(chan bool)
		go func() {
			ticker := time.Tick(1 * time.Millisecond)
			for {
				select {
				case <-initialPushes:
					// we have data or chan was closed, either way
					// interpret that as exit message
					return
				case <-ticker:
					cClient.Push(ctx, &cacher.PushRequest{Data: payload})
				}
			}
		}()

		hw := cacher.Hardware{}
		assert.NoError(t, w.RecvMsg(&hw))
		orig := hw.JSON
		close(initialPushes)

		// ok now that w.RecvMsg has processed at least one push we can run the actual test.
		// We need to accept the original response as the value because the initialPush go routine
		// could have snuck in a couple extra pushes while we get back from the w.RecvMsg call

		var expected string
		count := 0
		for count < 42 { // should be enough right?
			if expected == "" {
				expected = uuid.Must(uuid.NewRandom()).String()
				payload = fmt.Sprintf(`{"id": "%s", "ip": "%s", "hostname": "%s"}`, id, id, expected)
				_, err = cClient.Push(ctx, &cacher.PushRequest{Data: id + "=" + payload})
				assert.NoError(t, err)
			}

			assert.NoError(t, w.RecvMsg(&hw))
			if hw.JSON == orig {
				// this is an extra push from the initialPushes goroutine
				// no need to do anything special, a new payload should have
				// already been pushed so we'll just loop and try again
				continue
			}

			assert.Contains(t, hw.JSON, expected)
			hw.JSON = ""
			expected = ""
			count++
		}

		assert.Equal(t, 42, count)
	})
}
