package client

import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	"github.com/kubernetes-csi/csi-lib-utils/rpc"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	modifyrpc "github.com/awslabs/volume-modifier-for-k8s/pkg/rpc"
)

type Client interface {
	GetDriverName(context.Context) (string, error)

	SupportsVolumeModification(context.Context) error

	Modify(ctx context.Context, volumeID string, params, reqContext map[string]string) error

	CloseConnection()
}

func New(addr string, timeout time.Duration, metricsmanager metrics.CSIMetricsManager, enableControllerClient bool) (Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := connection.Connect(ctx, addr, metricsmanager, connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CSI driver: %w", err)
	}

	err = rpc.ProbeForever(ctx, conn, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed probing CSI driver: %w", err)
	}

	csiClient := &client{
		conn: conn,
	}

	if enableControllerClient {
		csiClient.ctrlClient = csi.NewControllerClient(conn)
	}

	return csiClient, nil
}

type client struct {
	conn       *grpc.ClientConn
	ctrlClient csi.ControllerClient
}

func (c *client) GetDriverName(ctx context.Context) (string, error) {
	return rpc.GetDriverName(ctx, c.conn)
}

func (c *client) SupportsVolumeModification(ctx context.Context) error {
	if c.ctrlClient != nil {
		caps, err := rpc.GetControllerCapabilities(ctx, c.conn)
		if err != nil {
			return fmt.Errorf("error getting controller capabilities: %v", err)
		}
		if !caps[csi.ControllerServiceCapability_RPC_MODIFY_VOLUME] {
			return fmt.Errorf("CSI driver does not support controller modify")
		}
		return nil
	}

	cc := modifyrpc.NewModifyClient(c.conn)
	req := &modifyrpc.GetCSIDriverModificationCapabilityRequest{}
	_, err := cc.GetCSIDriverModificationCapability(ctx, req)
	return err
}

func (c *client) Modify(ctx context.Context, volumeID string, params, reqContext map[string]string) error {
	if c.ctrlClient != nil {
		req := &csi.ControllerModifyVolumeRequest{
			VolumeId:          volumeID,
			Secrets:           reqContext,
			MutableParameters: params,
		}
		_, err := c.ctrlClient.ControllerModifyVolume(ctx, req)
		if err != nil {
			return err
		}
		return nil
	}

	cc := modifyrpc.NewModifyClient(c.conn)
	req := &modifyrpc.ModifyVolumePropertiesRequest{
		Name:       volumeID,
		Parameters: params,
		Context:    reqContext,
	}
	_, err := cc.ModifyVolumeProperties(ctx, req)
	if err == nil {
		klog.V(4).InfoS("Volume modification completed", "volumeID", volumeID)
	}
	return err
}

func (c *client) CloseConnection() {
	c.conn.Close()
}
