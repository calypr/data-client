package gecko

import "context"

func GetTypedConfig[T any](ctx context.Context, c GeckoInterface, configType ConfigType, configID string) (*T, error) {
	var cfg T
	if err := c.GetConfig(ctx, configType, configID, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func PutTypedConfig[T any](ctx context.Context, c GeckoInterface, configType ConfigType, configID string, cfg T) (*StatusResponse, error) {
	return c.PutConfig(ctx, configType, configID, cfg)
}

func DeleteTypedConfig(ctx context.Context, c GeckoInterface, configType ConfigType, configID string) (*StatusResponse, error) {
	return c.DeleteConfig(ctx, configType, configID)
}

func ListExplorerConfigs(ctx context.Context, c GeckoInterface) ([]string, error) {
	return c.ListConfigs(ctx, ConfigTypeExplorer)
}

func GetExplorerConfig(ctx context.Context, c GeckoInterface, configID string) (*Config, error) {
	return GetTypedConfig[Config](ctx, c, ConfigTypeExplorer, configID)
}

func PutExplorerConfig(ctx context.Context, c GeckoInterface, configID string, cfg Config) (*StatusResponse, error) {
	return PutTypedConfig(ctx, c, ConfigTypeExplorer, configID, cfg)
}

func DeleteExplorerConfig(ctx context.Context, c GeckoInterface, configID string) (*StatusResponse, error) {
	return DeleteTypedConfig(ctx, c, ConfigTypeExplorer, configID)
}

func GetProjectConfig(ctx context.Context, c GeckoInterface, configID string) (*ProjectConfig, error) {
	return GetTypedConfig[ProjectConfig](ctx, c, ConfigTypeProjects, configID)
}

func PutProjectConfig(ctx context.Context, c GeckoInterface, configID string, cfg ProjectConfig) (*StatusResponse, error) {
	normalized, err := validateProjectRepoURL(ctx, cfg.SrcRepo, "")
	if err != nil {
		return nil, err
	}
	cfg.SrcRepo = normalized
	return PutTypedConfig(ctx, c, ConfigTypeProjects, configID, cfg)
}

func DeleteProjectConfig(ctx context.Context, c GeckoInterface, configID string) (*StatusResponse, error) {
	return DeleteTypedConfig(ctx, c, ConfigTypeProjects, configID)
}

func GetAppCard(ctx context.Context, c GeckoInterface, projectID string) (*AppCard, error) {
	return c.GetAppCard(ctx, projectID)
}

func UpsertAppCard(ctx context.Context, c GeckoInterface, projectID string, card AppCard) (*StatusResponse, error) {
	return c.UpsertAppCard(ctx, projectID, card)
}

func DeleteAppCard(ctx context.Context, c GeckoInterface, projectID string) (*StatusResponse, error) {
	return c.DeleteAppCard(ctx, projectID)
}

func HealthCheck(ctx context.Context, c GeckoInterface) (string, error) {
	return c.HealthCheck(ctx)
}
