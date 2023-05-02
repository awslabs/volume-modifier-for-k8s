package client

import (
	"context"
	"fmt"
	"sync"
)

func NewFakeClient(
	name string,
	driverSupportsModification bool,
	modificationShouldFail bool,
) *FakeClient {
	return &FakeClient{
		name:                       name,
		driverSupportsModification: driverSupportsModification,
		modificationShouldFail:     modificationShouldFail,
	}
}

type FakeClient struct {
	name                       string
	driverSupportsModification bool
	modifyCalledMu             sync.Mutex
	modifyCalled               int
	modificationShouldFail     bool
	closed                     bool
	volumeID                   string
	params                     map[string]string
	reqContext                 map[string]string
}

func (f *FakeClient) GetDriverName(context.Context) (string, error) {
	return f.name, nil
}

func (f *FakeClient) SupportsVolumeModification(context.Context) error {
	if !f.driverSupportsModification {
		return fmt.Errorf("does not support modification")
	}
	return nil
}

func (f *FakeClient) Modify(ctx context.Context, volumeID string, params, reqContext map[string]string) error {
	f.modifyCalledMu.Lock()
	defer f.modifyCalledMu.Unlock()
	f.modifyCalled++
	f.volumeID = volumeID
	f.params = params
	f.reqContext = reqContext
	if f.modificationShouldFail {
		return fmt.Errorf("modification failed")
	}
	return nil
}

func (f *FakeClient) CloseConnection() {
	f.closed = true
}

func (f *FakeClient) GetVolumeName() string {
	return f.volumeID
}

func (f *FakeClient) GetParams() map[string]string {
	return f.params
}

func (f *FakeClient) GetReqContext() map[string]string {
	return f.reqContext
}

func (f *FakeClient) GetModifyCallCount() int {
	f.modifyCalledMu.Lock()
	defer f.modifyCalledMu.Unlock()
	return f.modifyCalled
}
