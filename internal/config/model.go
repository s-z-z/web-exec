// Package config provides the data model and persistence layer for route configurations.
package config

import (
	"time"
)

// RouteConfig represents a single route configuration with its associated
// shell script and working directory.
type RouteConfig struct {
	// ID is a unique identifier for the route (auto-generated, 8-char hex).
	ID string `json:"id"`
	// Path is the HTTP route path (e.g. "/deploy").
	Path string `json:"path"`
	// WorkDir is the absolute path to the working directory.
	WorkDir string `json:"workDir"`
	// Script is the shell script content executed when the route is triggered.
	Script string `json:"script"`
	// CreatedAt is the timestamp when the route was created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is the timestamp when the route was last updated.
	UpdatedAt time.Time `json:"updatedAt"`
}
