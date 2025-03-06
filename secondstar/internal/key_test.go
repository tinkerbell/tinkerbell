package internal

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	gssh "github.com/gliderlabs/ssh"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	xssh "golang.org/x/crypto/ssh"
)

// mockReader implements the Reader interface for testing.
type mockReader struct {
	readBMCMachineFunc func(ctx context.Context, name string) (*data.BMCMachine, error)
}

func (m *mockReader) ReadBMCMachine(ctx context.Context, name string) (*data.BMCMachine, error) {
	return m.readBMCMachineFunc(ctx, name)
}

// mockContext implements gliderlabs.Context interface.
type mockContext struct {
	context.Context
	user          string
	clientVersion string
	serverVersion string
	session       gssh.Session
	values        map[interface{}]interface{}
	remoteAddr    net.Addr
	localAddr     net.Addr
	perms         *xssh.Permissions
	mu            sync.Mutex
}

func newMockContext() *mockContext {
	ctx := context.Background()
	return &mockContext{
		Context: ctx,
		values:  make(map[interface{}]interface{}),
	}
}

func (m *mockContext) User() string {
	return m.user
}

func (m *mockContext) ClientVersion() string {
	return m.clientVersion
}

func (m *mockContext) ServerVersion() string {
	return m.serverVersion
}

func (m *mockContext) Session() gssh.Session {
	return m.session
}

func (m *mockContext) SetValue(key interface{}, value interface{}) {
	m.values[key] = value
}

func (m *mockContext) Value(key interface{}) interface{} {
	if val, ok := m.values[key]; ok {
		return val
	}
	return nil
}

func (m *mockContext) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *mockContext) LocalAddr() net.Addr {
	return m.localAddr
}

func (m *mockContext) Permissions() *gssh.Permissions {
	return &gssh.Permissions{Permissions: m.perms}
}

func (m *mockContext) SessionID() string {
	return "test-session-id"
}

func (m *mockContext) Lock() {
	m.mu.Lock()
}

func (m *mockContext) Unlock() {
	m.mu.Unlock()
}

// mockPublicKey implements gliderlabs.PublicKey interface by embedding crypto.PublicKey.
type mockPublicKey struct {
	data string
}

func (m *mockPublicKey) Type() string {
	return "ssh-ed25519"
}

func (m *mockPublicKey) Marshal() []byte {
	out, _, _, _, err := xssh.ParseAuthorizedKey([]byte(m.data)) //nolint:dogsled // Not important for tests.
	if err != nil {
		return nil
	}
	return out.Marshal()
}

func (m *mockPublicKey) Verify([]byte, *xssh.Signature) error {
	return nil
}

func TestPubkeyAuth(t *testing.T) {
	tests := map[string]struct {
		machine          *data.BMCMachine
		readMachineError error
		expectedResult   bool
	}{
		"valid key found": {
			machine: &data.BMCMachine{
				Host:          "127.0.0.1",
				User:          "test-user",
				Port:          22,
				Pass:          "test-pass",
				SSHPublicKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFnF/FAvw9XpuMFPtwKkDeOO/YnTs9P5HX1CCecFUyvc"},
			},
			expectedResult: true,
		},
		"machine is nil, no key found": {},
		"machine is not nil, no key found": {
			machine: &data.BMCMachine{},
		},
		"error reading machine": {
			readMachineError: errors.New("error reading machine"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &mockReader{
				readBMCMachineFunc: func(context.Context, string) (*data.BMCMachine, error) {
					return tt.machine, tt.readMachineError
				},
			}
			authFunc := PubkeyAuth(reader, logr.Discard())

			key := &mockPublicKey{
				data: func() string {
					if tt.machine != nil && len(tt.machine.SSHPublicKeys) > 0 {
						return tt.machine.SSHPublicKeys[0]
					}
					return ""
				}(),
			}
			result := authFunc(newMockContext(), key)

			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}
