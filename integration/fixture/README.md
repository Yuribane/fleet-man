# fleet-man integration fixture

Minimal devcontainer fixture used by the integration test suite.

The suite copies this directory into a throwaway git repo and points
`fleet up --repo file://<path>` at it. A lightweight debian-base image
keeps test runs fast compared to booting the full fleet-man devcontainer
(which pulls Go + node + docker-in-docker features).
