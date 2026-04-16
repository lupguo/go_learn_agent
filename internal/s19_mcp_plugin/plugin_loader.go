package s19_mcp_plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginManifest is the structure of .claude-plugin/plugin.json.
type PluginManifest struct {
	Name       string                       `json:"name"`
	MCPServers map[string]MCPServerConfig   `json:"mcpServers"`
}

// MCPServerConfig is a server entry in the plugin manifest.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// PluginLoader scans directories for .claude-plugin/plugin.json manifests.
type PluginLoader struct {
	searchDirs []string
	plugins    map[string]PluginManifest
}

func NewPluginLoader(searchDirs []string) *PluginLoader {
	return &PluginLoader{
		searchDirs: searchDirs,
		plugins:    make(map[string]PluginManifest),
	}
}

// Scan looks for plugin manifests and returns names of found plugins.
func (l *PluginLoader) Scan() []string {
	var found []string
	for _, dir := range l.searchDirs {
		manifestPath := filepath.Join(dir, ".claude-plugin", "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			fmt.Fprintf(os.Stderr, "[Plugin] Failed to parse %s: %v\n", manifestPath, err)
			continue
		}
		if manifest.Name == "" {
			manifest.Name = filepath.Base(dir)
		}
		l.plugins[manifest.Name] = manifest
		found = append(found, manifest.Name)
	}
	return found
}

// GetMCPServers extracts MCP server configs from loaded plugins.
// Keys are "pluginName__serverName".
func (l *PluginLoader) GetMCPServers() map[string]MCPServerConfig {
	servers := make(map[string]MCPServerConfig)
	for pluginName, manifest := range l.plugins {
		for serverName, config := range manifest.MCPServers {
			key := fmt.Sprintf("%s__%s", pluginName, serverName)
			servers[key] = config
		}
	}
	return servers
}
