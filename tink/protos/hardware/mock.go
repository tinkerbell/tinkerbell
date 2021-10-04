// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package hardware

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Ensure, that HardwareServiceClientMock does implement HardwareServiceClient.
// If this is not the case, regenerate this file with moq.
var _ HardwareServiceClient = &HardwareServiceClientMock{}

// HardwareServiceClientMock is a mock implementation of HardwareServiceClient.
//
//     func TestSomethingThatUsesHardwareServiceClient(t *testing.T) {
//
//         // make and configure a mocked HardwareServiceClient
//         mockedHardwareServiceClient := &HardwareServiceClientMock{
//             AllFunc: func(ctx context.Context, in *Empty, opts ...grpc.CallOption) (HardwareService_AllClient, error) {
// 	               panic("mock out the All method")
//             },
//             ByIDFunc: func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
// 	               panic("mock out the ByID method")
//             },
//             ByIPFunc: func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
// 	               panic("mock out the ByIP method")
//             },
//             ByMACFunc: func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
// 	               panic("mock out the ByMAC method")
//             },
//             DeleteFunc: func(ctx context.Context, in *DeleteRequest, opts ...grpc.CallOption) (*Empty, error) {
// 	               panic("mock out the Delete method")
//             },
//             DeprecatedWatchFunc: func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (HardwareService_DeprecatedWatchClient, error) {
// 	               panic("mock out the DeprecatedWatch method")
//             },
//             PushFunc: func(ctx context.Context, in *PushRequest, opts ...grpc.CallOption) (*Empty, error) {
// 	               panic("mock out the Push method")
//             },
//         }
//
//         // use mockedHardwareServiceClient in code that requires HardwareServiceClient
//         // and then make assertions.
//
//     }
type HardwareServiceClientMock struct {
	// AllFunc mocks the All method.
	AllFunc func(ctx context.Context, in *Empty, opts ...grpc.CallOption) (HardwareService_AllClient, error)

	// ByIDFunc mocks the ByID method.
	ByIDFunc func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error)

	// ByIPFunc mocks the ByIP method.
	ByIPFunc func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error)

	// ByMACFunc mocks the ByMAC method.
	ByMACFunc func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error)

	// DeleteFunc mocks the Delete method.
	DeleteFunc func(ctx context.Context, in *DeleteRequest, opts ...grpc.CallOption) (*Empty, error)

	// DeprecatedWatchFunc mocks the DeprecatedWatch method.
	DeprecatedWatchFunc func(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (HardwareService_DeprecatedWatchClient, error)

	// PushFunc mocks the Push method.
	PushFunc func(ctx context.Context, in *PushRequest, opts ...grpc.CallOption) (*Empty, error)

	// calls tracks calls to the methods.
	calls struct {
		// All holds details about calls to the All method.
		All []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *Empty
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// ByID holds details about calls to the ByID method.
		ByID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *GetRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// ByIP holds details about calls to the ByIP method.
		ByIP []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *GetRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// ByMAC holds details about calls to the ByMAC method.
		ByMAC []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *GetRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// Delete holds details about calls to the Delete method.
		Delete []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *DeleteRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// DeprecatedWatch holds details about calls to the DeprecatedWatch method.
		DeprecatedWatch []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *GetRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
		// Push holds details about calls to the Push method.
		Push []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// In is the in argument value.
			In *PushRequest
			// Opts is the opts argument value.
			Opts []grpc.CallOption
		}
	}
	lockAll             sync.RWMutex
	lockByID            sync.RWMutex
	lockByIP            sync.RWMutex
	lockByMAC           sync.RWMutex
	lockDelete          sync.RWMutex
	lockDeprecatedWatch sync.RWMutex
	lockPush            sync.RWMutex
}

// All calls AllFunc.
func (mock *HardwareServiceClientMock) All(ctx context.Context, in *Empty, opts ...grpc.CallOption) (HardwareService_AllClient, error) {
	if mock.AllFunc == nil {
		panic("HardwareServiceClientMock.AllFunc: method is nil but HardwareServiceClient.All was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *Empty
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockAll.Lock()
	mock.calls.All = append(mock.calls.All, callInfo)
	mock.lockAll.Unlock()
	return mock.AllFunc(ctx, in, opts...)
}

// AllCalls gets all the calls that were made to All.
// Check the length with:
//     len(mockedHardwareServiceClient.AllCalls())
func (mock *HardwareServiceClientMock) AllCalls() []struct {
	Ctx  context.Context
	In   *Empty
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *Empty
		Opts []grpc.CallOption
	}
	mock.lockAll.RLock()
	calls = mock.calls.All
	mock.lockAll.RUnlock()
	return calls
}

// ByID calls ByIDFunc.
func (mock *HardwareServiceClientMock) ByID(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
	if mock.ByIDFunc == nil {
		panic("HardwareServiceClientMock.ByIDFunc: method is nil but HardwareServiceClient.ByID was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockByID.Lock()
	mock.calls.ByID = append(mock.calls.ByID, callInfo)
	mock.lockByID.Unlock()
	return mock.ByIDFunc(ctx, in, opts...)
}

// ByIDCalls gets all the calls that were made to ByID.
// Check the length with:
//     len(mockedHardwareServiceClient.ByIDCalls())
func (mock *HardwareServiceClientMock) ByIDCalls() []struct {
	Ctx  context.Context
	In   *GetRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}
	mock.lockByID.RLock()
	calls = mock.calls.ByID
	mock.lockByID.RUnlock()
	return calls
}

// ByIP calls ByIPFunc.
func (mock *HardwareServiceClientMock) ByIP(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
	if mock.ByIPFunc == nil {
		panic("HardwareServiceClientMock.ByIPFunc: method is nil but HardwareServiceClient.ByIP was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockByIP.Lock()
	mock.calls.ByIP = append(mock.calls.ByIP, callInfo)
	mock.lockByIP.Unlock()
	return mock.ByIPFunc(ctx, in, opts...)
}

// ByIPCalls gets all the calls that were made to ByIP.
// Check the length with:
//     len(mockedHardwareServiceClient.ByIPCalls())
func (mock *HardwareServiceClientMock) ByIPCalls() []struct {
	Ctx  context.Context
	In   *GetRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}
	mock.lockByIP.RLock()
	calls = mock.calls.ByIP
	mock.lockByIP.RUnlock()
	return calls
}

// ByMAC calls ByMACFunc.
func (mock *HardwareServiceClientMock) ByMAC(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (*Hardware, error) {
	if mock.ByMACFunc == nil {
		panic("HardwareServiceClientMock.ByMACFunc: method is nil but HardwareServiceClient.ByMAC was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockByMAC.Lock()
	mock.calls.ByMAC = append(mock.calls.ByMAC, callInfo)
	mock.lockByMAC.Unlock()
	return mock.ByMACFunc(ctx, in, opts...)
}

// ByMACCalls gets all the calls that were made to ByMAC.
// Check the length with:
//     len(mockedHardwareServiceClient.ByMACCalls())
func (mock *HardwareServiceClientMock) ByMACCalls() []struct {
	Ctx  context.Context
	In   *GetRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}
	mock.lockByMAC.RLock()
	calls = mock.calls.ByMAC
	mock.lockByMAC.RUnlock()
	return calls
}

// Delete calls DeleteFunc.
func (mock *HardwareServiceClientMock) Delete(ctx context.Context, in *DeleteRequest, opts ...grpc.CallOption) (*Empty, error) {
	if mock.DeleteFunc == nil {
		panic("HardwareServiceClientMock.DeleteFunc: method is nil but HardwareServiceClient.Delete was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *DeleteRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockDelete.Lock()
	mock.calls.Delete = append(mock.calls.Delete, callInfo)
	mock.lockDelete.Unlock()
	return mock.DeleteFunc(ctx, in, opts...)
}

// DeleteCalls gets all the calls that were made to Delete.
// Check the length with:
//     len(mockedHardwareServiceClient.DeleteCalls())
func (mock *HardwareServiceClientMock) DeleteCalls() []struct {
	Ctx  context.Context
	In   *DeleteRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *DeleteRequest
		Opts []grpc.CallOption
	}
	mock.lockDelete.RLock()
	calls = mock.calls.Delete
	mock.lockDelete.RUnlock()
	return calls
}

// DeprecatedWatch calls DeprecatedWatchFunc.
func (mock *HardwareServiceClientMock) DeprecatedWatch(ctx context.Context, in *GetRequest, opts ...grpc.CallOption) (HardwareService_DeprecatedWatchClient, error) {
	if mock.DeprecatedWatchFunc == nil {
		panic("HardwareServiceClientMock.DeprecatedWatchFunc: method is nil but HardwareServiceClient.DeprecatedWatch was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockDeprecatedWatch.Lock()
	mock.calls.DeprecatedWatch = append(mock.calls.DeprecatedWatch, callInfo)
	mock.lockDeprecatedWatch.Unlock()
	return mock.DeprecatedWatchFunc(ctx, in, opts...)
}

// DeprecatedWatchCalls gets all the calls that were made to DeprecatedWatch.
// Check the length with:
//     len(mockedHardwareServiceClient.DeprecatedWatchCalls())
func (mock *HardwareServiceClientMock) DeprecatedWatchCalls() []struct {
	Ctx  context.Context
	In   *GetRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *GetRequest
		Opts []grpc.CallOption
	}
	mock.lockDeprecatedWatch.RLock()
	calls = mock.calls.DeprecatedWatch
	mock.lockDeprecatedWatch.RUnlock()
	return calls
}

// Push calls PushFunc.
func (mock *HardwareServiceClientMock) Push(ctx context.Context, in *PushRequest, opts ...grpc.CallOption) (*Empty, error) {
	if mock.PushFunc == nil {
		panic("HardwareServiceClientMock.PushFunc: method is nil but HardwareServiceClient.Push was just called")
	}
	callInfo := struct {
		Ctx  context.Context
		In   *PushRequest
		Opts []grpc.CallOption
	}{
		Ctx:  ctx,
		In:   in,
		Opts: opts,
	}
	mock.lockPush.Lock()
	mock.calls.Push = append(mock.calls.Push, callInfo)
	mock.lockPush.Unlock()
	return mock.PushFunc(ctx, in, opts...)
}

// PushCalls gets all the calls that were made to Push.
// Check the length with:
//     len(mockedHardwareServiceClient.PushCalls())
func (mock *HardwareServiceClientMock) PushCalls() []struct {
	Ctx  context.Context
	In   *PushRequest
	Opts []grpc.CallOption
} {
	var calls []struct {
		Ctx  context.Context
		In   *PushRequest
		Opts []grpc.CallOption
	}
	mock.lockPush.RLock()
	calls = mock.calls.Push
	mock.lockPush.RUnlock()
	return calls
}

// Ensure, that HardwareService_AllClientMock does implement HardwareService_AllClient.
// If this is not the case, regenerate this file with moq.
var _ HardwareService_AllClient = &HardwareService_AllClientMock{}

// HardwareService_AllClientMock is a mock implementation of HardwareService_AllClient.
//
//     func TestSomethingThatUsesHardwareService_AllClient(t *testing.T) {
//
//         // make and configure a mocked HardwareService_AllClient
//         mockedHardwareService_AllClient := &HardwareService_AllClientMock{
//             CloseSendFunc: func() error {
// 	               panic("mock out the CloseSend method")
//             },
//             ContextFunc: func() context.Context {
// 	               panic("mock out the Context method")
//             },
//             HeaderFunc: func() (metadata.MD, error) {
// 	               panic("mock out the Header method")
//             },
//             RecvFunc: func() (*Hardware, error) {
// 	               panic("mock out the Recv method")
//             },
//             RecvMsgFunc: func(m interface{}) error {
// 	               panic("mock out the RecvMsg method")
//             },
//             SendMsgFunc: func(m interface{}) error {
// 	               panic("mock out the SendMsg method")
//             },
//             TrailerFunc: func() metadata.MD {
// 	               panic("mock out the Trailer method")
//             },
//         }
//
//         // use mockedHardwareService_AllClient in code that requires HardwareService_AllClient
//         // and then make assertions.
//
//     }
type HardwareService_AllClientMock struct {
	// CloseSendFunc mocks the CloseSend method.
	CloseSendFunc func() error

	// ContextFunc mocks the Context method.
	ContextFunc func() context.Context

	// HeaderFunc mocks the Header method.
	HeaderFunc func() (metadata.MD, error)

	// RecvFunc mocks the Recv method.
	RecvFunc func() (*Hardware, error)

	// RecvMsgFunc mocks the RecvMsg method.
	RecvMsgFunc func(m interface{}) error

	// SendMsgFunc mocks the SendMsg method.
	SendMsgFunc func(m interface{}) error

	// TrailerFunc mocks the Trailer method.
	TrailerFunc func() metadata.MD

	// calls tracks calls to the methods.
	calls struct {
		// CloseSend holds details about calls to the CloseSend method.
		CloseSend []struct{}
		// Context holds details about calls to the Context method.
		Context []struct{}
		// Header holds details about calls to the Header method.
		Header []struct{}
		// Recv holds details about calls to the Recv method.
		Recv []struct{}
		// RecvMsg holds details about calls to the RecvMsg method.
		RecvMsg []struct {
			// M is the m argument value.
			M interface{}
		}
		// SendMsg holds details about calls to the SendMsg method.
		SendMsg []struct {
			// M is the m argument value.
			M interface{}
		}
		// Trailer holds details about calls to the Trailer method.
		Trailer []struct{}
	}
	lockCloseSend sync.RWMutex
	lockContext   sync.RWMutex
	lockHeader    sync.RWMutex
	lockRecv      sync.RWMutex
	lockRecvMsg   sync.RWMutex
	lockSendMsg   sync.RWMutex
	lockTrailer   sync.RWMutex
}

// CloseSend calls CloseSendFunc.
func (mock *HardwareService_AllClientMock) CloseSend() error {
	if mock.CloseSendFunc == nil {
		panic("HardwareService_AllClientMock.CloseSendFunc: method is nil but HardwareService_AllClient.CloseSend was just called")
	}
	callInfo := struct{}{}
	mock.lockCloseSend.Lock()
	mock.calls.CloseSend = append(mock.calls.CloseSend, callInfo)
	mock.lockCloseSend.Unlock()
	return mock.CloseSendFunc()
}

// CloseSendCalls gets all the calls that were made to CloseSend.
// Check the length with:
//     len(mockedHardwareService_AllClient.CloseSendCalls())
func (mock *HardwareService_AllClientMock) CloseSendCalls() []struct{} {
	var calls []struct{}
	mock.lockCloseSend.RLock()
	calls = mock.calls.CloseSend
	mock.lockCloseSend.RUnlock()
	return calls
}

// Context calls ContextFunc.
func (mock *HardwareService_AllClientMock) Context() context.Context {
	if mock.ContextFunc == nil {
		panic("HardwareService_AllClientMock.ContextFunc: method is nil but HardwareService_AllClient.Context was just called")
	}
	callInfo := struct{}{}
	mock.lockContext.Lock()
	mock.calls.Context = append(mock.calls.Context, callInfo)
	mock.lockContext.Unlock()
	return mock.ContextFunc()
}

// ContextCalls gets all the calls that were made to Context.
// Check the length with:
//     len(mockedHardwareService_AllClient.ContextCalls())
func (mock *HardwareService_AllClientMock) ContextCalls() []struct{} {
	var calls []struct{}
	mock.lockContext.RLock()
	calls = mock.calls.Context
	mock.lockContext.RUnlock()
	return calls
}

// Header calls HeaderFunc.
func (mock *HardwareService_AllClientMock) Header() (metadata.MD, error) {
	if mock.HeaderFunc == nil {
		panic("HardwareService_AllClientMock.HeaderFunc: method is nil but HardwareService_AllClient.Header was just called")
	}
	callInfo := struct{}{}
	mock.lockHeader.Lock()
	mock.calls.Header = append(mock.calls.Header, callInfo)
	mock.lockHeader.Unlock()
	return mock.HeaderFunc()
}

// HeaderCalls gets all the calls that were made to Header.
// Check the length with:
//     len(mockedHardwareService_AllClient.HeaderCalls())
func (mock *HardwareService_AllClientMock) HeaderCalls() []struct{} {
	var calls []struct{}
	mock.lockHeader.RLock()
	calls = mock.calls.Header
	mock.lockHeader.RUnlock()
	return calls
}

// Recv calls RecvFunc.
func (mock *HardwareService_AllClientMock) Recv() (*Hardware, error) {
	if mock.RecvFunc == nil {
		panic("HardwareService_AllClientMock.RecvFunc: method is nil but HardwareService_AllClient.Recv was just called")
	}
	callInfo := struct{}{}
	mock.lockRecv.Lock()
	mock.calls.Recv = append(mock.calls.Recv, callInfo)
	mock.lockRecv.Unlock()
	return mock.RecvFunc()
}

// RecvCalls gets all the calls that were made to Recv.
// Check the length with:
//     len(mockedHardwareService_AllClient.RecvCalls())
func (mock *HardwareService_AllClientMock) RecvCalls() []struct{} {
	var calls []struct{}
	mock.lockRecv.RLock()
	calls = mock.calls.Recv
	mock.lockRecv.RUnlock()
	return calls
}

// RecvMsg calls RecvMsgFunc.
func (mock *HardwareService_AllClientMock) RecvMsg(m interface{}) error {
	if mock.RecvMsgFunc == nil {
		panic("HardwareService_AllClientMock.RecvMsgFunc: method is nil but HardwareService_AllClient.RecvMsg was just called")
	}
	callInfo := struct {
		M interface{}
	}{
		M: m,
	}
	mock.lockRecvMsg.Lock()
	mock.calls.RecvMsg = append(mock.calls.RecvMsg, callInfo)
	mock.lockRecvMsg.Unlock()
	return mock.RecvMsgFunc(m)
}

// RecvMsgCalls gets all the calls that were made to RecvMsg.
// Check the length with:
//     len(mockedHardwareService_AllClient.RecvMsgCalls())
func (mock *HardwareService_AllClientMock) RecvMsgCalls() []struct {
	M interface{}
} {
	var calls []struct {
		M interface{}
	}
	mock.lockRecvMsg.RLock()
	calls = mock.calls.RecvMsg
	mock.lockRecvMsg.RUnlock()
	return calls
}

// SendMsg calls SendMsgFunc.
func (mock *HardwareService_AllClientMock) SendMsg(m interface{}) error {
	if mock.SendMsgFunc == nil {
		panic("HardwareService_AllClientMock.SendMsgFunc: method is nil but HardwareService_AllClient.SendMsg was just called")
	}
	callInfo := struct {
		M interface{}
	}{
		M: m,
	}
	mock.lockSendMsg.Lock()
	mock.calls.SendMsg = append(mock.calls.SendMsg, callInfo)
	mock.lockSendMsg.Unlock()
	return mock.SendMsgFunc(m)
}

// SendMsgCalls gets all the calls that were made to SendMsg.
// Check the length with:
//     len(mockedHardwareService_AllClient.SendMsgCalls())
func (mock *HardwareService_AllClientMock) SendMsgCalls() []struct {
	M interface{}
} {
	var calls []struct {
		M interface{}
	}
	mock.lockSendMsg.RLock()
	calls = mock.calls.SendMsg
	mock.lockSendMsg.RUnlock()
	return calls
}

// Trailer calls TrailerFunc.
func (mock *HardwareService_AllClientMock) Trailer() metadata.MD {
	if mock.TrailerFunc == nil {
		panic("HardwareService_AllClientMock.TrailerFunc: method is nil but HardwareService_AllClient.Trailer was just called")
	}
	callInfo := struct{}{}
	mock.lockTrailer.Lock()
	mock.calls.Trailer = append(mock.calls.Trailer, callInfo)
	mock.lockTrailer.Unlock()
	return mock.TrailerFunc()
}

// TrailerCalls gets all the calls that were made to Trailer.
// Check the length with:
//     len(mockedHardwareService_AllClient.TrailerCalls())
func (mock *HardwareService_AllClientMock) TrailerCalls() []struct{} {
	var calls []struct{}
	mock.lockTrailer.RLock()
	calls = mock.calls.Trailer
	mock.lockTrailer.RUnlock()
	return calls
}
