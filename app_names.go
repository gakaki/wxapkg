package main

import (
	"context"
	"strings"
	"time"

	"github.com/wux1an/wxapkg/wechat"
)

type appNameLookup struct {
	UUID               string
	Key                string
	Provider           string
	ProviderID         string
	FallbackProvider   string
	FallbackProviderID string
}

var appNameLookupSlots = make(chan struct{}, 4)

// enrichAppNames must be called from Window.Update. Cached names are applied
// immediately; uncached public profile names are fetched in the background.
func (a *app) enrichAppNames() {
	if a.win == nil {
		return
	}
	if a.nameCache == nil {
		a.nameCache = make(map[string]string)
	}
	if a.nameLookups == nil {
		a.nameLookups = make(map[string]struct{})
	}

	var lookups []appNameLookup
	changed := false
	for index := range a.items {
		item := &a.items[index]
		if strings.TrimSpace(item.Name) != "" || item.NameProvider == "" || item.NameProviderID == "" {
			continue
		}
		key := appNameCacheKey(item.NameProvider, item.NameProviderID)
		if cached := strings.TrimSpace(a.nameCache[key]); cached != "" {
			item.Name = cached
			changed = true
			continue
		}
		if _, running := a.nameLookups[key]; running {
			continue
		}
		a.nameLookups[key] = struct{}{}
		lookups = append(lookups, appNameLookup{
			UUID:               item.UUID,
			Key:                key,
			Provider:           item.NameProvider,
			ProviderID:         item.NameProviderID,
			FallbackProvider:   item.FallbackNameProvider,
			FallbackProviderID: item.FallbackNameProviderID,
		})
	}
	if changed {
		a.savePreferences()
	}
	for _, lookup := range lookups {
		lookup := lookup
		debugLog("name lookup queued: provider=%s id=%s", lookup.Provider, lookup.ProviderID)
		go a.fetchAppName(lookup)
	}
}

func (a *app) fetchAppName(lookup appNameLookup) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	select {
	case appNameLookupSlots <- struct{}{}:
		defer func() { <-appNameLookupSlots }()
	case <-ctx.Done():
		if a.win != nil {
			a.win.Update(func() { delete(a.nameLookups, lookup.Key) })
		}
		return
	}
	name, err := wechat.LookupProviderName(ctx, lookup.Provider, lookup.ProviderID)
	if err != nil && lookup.FallbackProvider != "" && lookup.FallbackProviderID != "" {
		debugLog("primary name lookup failed, trying fallback: provider=%s id=%s error=%v",
			lookup.Provider, lookup.ProviderID, err)
		name, err = wechat.LookupProviderName(
			ctx,
			lookup.FallbackProvider,
			lookup.FallbackProviderID,
		)
	}
	if err != nil {
		debugLog("name lookup failed: provider=%s id=%s error=%v", lookup.Provider, lookup.ProviderID, err)
	} else {
		debugLog("name lookup succeeded: provider=%s id=%s name=%s", lookup.Provider, lookup.ProviderID, name)
	}

	if a.win == nil {
		return
	}
	a.win.Update(func() {
		delete(a.nameLookups, lookup.Key)
		if err != nil || strings.TrimSpace(name) == "" {
			return
		}
		found := false
		for index := range a.items {
			if a.items[index].UUID != lookup.UUID {
				continue
			}
			found = true
			if strings.TrimSpace(a.items[index].Name) == "" {
				a.items[index].Name = name
			} else {
				name = a.items[index].Name
			}
			break
		}
		if !found {
			return
		}
		a.nameCache[lookup.Key] = name
		a.savePreferences()
	})
}

func appNameCacheKey(provider, providerID string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + ":" + strings.TrimSpace(providerID)
}
