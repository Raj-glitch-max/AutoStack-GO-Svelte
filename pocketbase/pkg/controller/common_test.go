package controller

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/security"
)

func mockAuthRecord(app core.App, collectionName string) (*models.Record, error) {
	collection, err := app.Dao().FindCollectionByNameOrId(collectionName)
	if err != nil { return nil, err }
	record := models.NewRecord(collection)
	record.Set("username", "test_" + security.RandomString(5))
	record.Set("email", "test_" + security.RandomString(5) + "@example.com")
	record.Set("password", "1234567890")
	record.Set("tokenKey", security.RandomString(50))
	if err := app.Dao().SaveRecord(record); err != nil { return nil, err }
	return record, nil
}
