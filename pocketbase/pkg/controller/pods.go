package controller

import (
	"net/http"

	"github.com/Raj-glitch-max/autostack/pkg/k8s"
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase"
)

func HandlePodDelete(c echo.Context, app *pocketbase.PocketBase, projectId string, podName string) error {

	namespace := projectId
	if namespace == "" {
		return c.JSON(http.StatusBadRequest, "projectId is required")
	}

	err := k8s.DeletePod(namespace, podName)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, "Pod deleted")
}
