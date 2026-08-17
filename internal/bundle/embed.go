package bundle

import "embed"

//go:embed viewer/* viewer/icons/*
var viewerFS embed.FS
