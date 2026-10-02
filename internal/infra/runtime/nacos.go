package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
	"github.com/ealink1/navi-fyne/internal/upstream/nacos"
)

type nacosClient struct{ client nacos.Client }
type nacosCommand struct {
	Op        string `json:"op"`
	Namespace string `json:"namespace"`
	Group     string `json:"group"`
	DataID    string `json:"dataId"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	Service   string `json:"service"`
	Page      int    `json:"page"`
}

func openNacos(ctx context.Context, cfg connection.ConnectionConfig) (Client, error) {
	client := nacos.NewClient()
	connectCtx, cancel := connectionContext(ctx, cfg)
	defer cancel()
	var err error
	if connector, ok := client.(interface {
		ConnectContext(context.Context, connection.ConnectionConfig) error
	}); ok {
		err = connector.ConnectContext(connectCtx, cfg)
	} else {
		err = client.Connect(cfg)
	}
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &nacosClient{client: client}, nil
}
func (c *nacosClient) Close() error { return c.client.Close() }
func (c *nacosClient) Scopes(ctx context.Context) ([]string, error) {
	namespaces, err := c.client.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	result := []string{""}
	for _, namespace := range namespaces {
		if namespace.ID != "" {
			result = append(result, namespace.ID)
		}
	}
	return result, nil
}
func (c *nacosClient) Objects(ctx context.Context, scope string) ([]domain.Object, error) {
	page, err := c.client.SearchConfigs(ctx, nacos.ConfigQuery{NamespaceID: scope, PageNo: 1, PageSize: 100, Search: "blur"})
	if err != nil {
		return nil, err
	}
	result := make([]domain.Object, 0, len(page.PageItems))
	for _, item := range page.PageItems {
		result = append(result, domain.Object{Name: item.DataID, Kind: item.Group, Scope: scope})
	}
	return result, nil
}
func (c *nacosClient) Schema(ctx context.Context, scope, name string) (string, error) {
	return "", errors.New("select a configuration and use Get content to supply its group")
}
func (c *nacosClient) Execute(ctx context.Context, e domain.Execution) ([]domain.Result, error) {
	var command nacosCommand
	if err := json.Unmarshal([]byte(e.Text), &command); err != nil {
		return nil, err
	}
	if command.Group == "" {
		command.Group = "DEFAULT_GROUP"
	}
	if command.Namespace == "" {
		command.Namespace = e.Scope
	}
	if command.Page < 1 {
		command.Page = 1
	}
	switch command.Op {
	case "namespaces":
		value, err := c.client.ListNamespaces(ctx)
		if err != nil {
			return nil, err
		}
		return JSONResult(value)
	case "configs":
		value, err := c.client.SearchConfigs(ctx, nacos.ConfigQuery{NamespaceID: command.Namespace, Group: command.Group, DataID: command.DataID, PageNo: command.Page, PageSize: 100, Search: "blur"})
		if err != nil {
			return nil, err
		}
		return JSONResult(value.PageItems)
	case "get":
		value, err := c.client.GetConfig(ctx, command.Namespace, command.Group, command.DataID)
		if err != nil {
			return nil, err
		}
		return JSONResult(value)
	case "services":
		value, err := c.client.ListServices(ctx, nacos.ServiceQuery{NamespaceID: command.Namespace, PageNo: command.Page, PageSize: 100})
		if err != nil {
			return nil, err
		}
		return JSONResult(value)
	case "instances":
		value, err := c.client.ListInstances(ctx, nacos.InstanceQuery{NamespaceID: command.Namespace, ServiceName: command.Service, GroupName: command.Group})
		if err != nil {
			return nil, err
		}
		return JSONResult(value)
	case "publish":
		if !e.Write {
			return nil, domain.ErrReadOnly
		}
		err := c.client.PublishConfig(ctx, nacos.PublishRequest{NamespaceID: command.Namespace, Group: command.Group, DataID: command.DataID, Content: command.Content, Type: command.Type})
		return []domain.Result{{RowsAffected: 1, Messages: []string{"发布请求已接受；配置在服务端传播后可读取。写入不会自动重试。"}}}, err
	case "delete":
		if !e.Write {
			return nil, domain.ErrReadOnly
		}
		err := c.client.DeleteConfig(ctx, command.Namespace, command.Group, command.DataID)
		return []domain.Result{{RowsAffected: 1}}, err
	default:
		return nil, errors.New("Nacos operation must be namespaces/configs/get/services/instances/publish/delete")
	}
}
