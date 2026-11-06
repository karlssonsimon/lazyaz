package acapp

import (
	"context"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/cache"

	tea "charm.land/bubbletea/v2"
)

func fetchStoresCmd(svc *appconfig.Service, broker *cache.Broker[appconfig.Store], subscriptionID string, seed []appconfig.Store) tea.Cmd {
	cmd, _ := broker.Subscribe(subscriptionID, seed, func(ctx context.Context, send func([]appconfig.Store)) error {
		return svc.ListStores(ctx, subscriptionID, send)
	}, func(p cache.Page[appconfig.Store]) tea.Msg {
		return storesLoadedMsg{subscriptionID: subscriptionID, stores: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}

func settingsScope(store appconfig.Store) string {
	return cache.Key(store.SubscriptionID, store.Name)
}

func fetchSettingsCmd(svc *appconfig.Service, broker *cache.Broker[appconfig.Setting], store appconfig.Store, seed []appconfig.Setting) tea.Cmd {
	cmd, _ := broker.Subscribe(settingsScope(store), seed, func(ctx context.Context, send func([]appconfig.Setting)) error {
		return svc.ListSettings(ctx, store, "", "", send)
	}, func(p cache.Page[appconfig.Setting]) tea.Msg {
		return settingsLoadedMsg{store: store, settings: p.Items, done: p.Done, err: p.Err, next: p.Next}
	})
	return cmd
}
