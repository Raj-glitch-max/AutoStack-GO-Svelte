package controller

import (
	"net/http"

	"github.com/janlauber/one-click/pkg/k8s"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
)

// HandleClusterInfo returns Kubernetes cluster info when the operator
// is connected to a cluster.  In LOCAL RUNTIME MODE (no kubeconfig, no
// in-cluster env), the K8s clients are nil and this endpoint returns
// a 200 with an empty/disabled payload — never a 400.  The frontend
// uses the absence of cluster data to decide whether to show K8s tabs.
func HandleClusterInfo(c echo.Context, app *pocketbase.PocketBase) error {
	if k8s.Clientset == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"enabled":            false,
			"mode":               "local-runtime",
			"reason":             "kubernetes disabled; runtime engine uses local Docker",
			"storage_classes":    []interface{}{},
			"ingress_classes":    []interface{}{},
			"namespaces":         []interface{}{},
			"server_version":     "",
		})
	}

	clusterInfo, err := k8s.GetClusterInfo()
	if err != nil {
		// Don't 500 the operator UI just because cluster fetch hiccupped —
		// surface the issue but keep the shape stable.
		return c.JSON(http.StatusOK, map[string]interface{}{
			"enabled": false,
			"mode":    "error",
			"reason":  err.Error(),
		})
	}
	return c.JSON(http.StatusOK, clusterInfo)
}
