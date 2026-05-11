package cost

// test_setup_test.go bootstraps the application-specific PocketBase collections that
// tests.NewTestApp() does not seed (it starts with a blank schema).
//
// This follows the established pattern from pkg/aws/pricing_test_setup.go.
// Any test that performs DB round-trips against custom collections must call
// the relevant bootstrap function before using those collections.

import (
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
)

// appWithDao is the minimal interface satisfied by both *pocketbase.PocketBase
// and *tests.TestApp — avoiding import-cycle issues in tests.
type appWithDao interface {
	Dao() *daos.Dao
}

// bootstrapDeploymentsCollection creates the deployments collection.
// This is the k8s-style deployment collection used by cost alert tests.
func bootstrapDeploymentsCollection(app appWithDao) error {
	collection := &models.Collection{
		Name: "deployments",
		Type: models.CollectionTypeBase,
		Schema: schema.NewSchema(
			&schema.SchemaField{Name: "name", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "user", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "status", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "blueprint", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "region", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "costAlertThreshold", Type: schema.FieldTypeNumber},
		),
	}
	return app.Dao().SaveCollection(collection)
}

// bootstrapAwsDeploymentsCollection creates the awsDeployments collection.
func bootstrapAwsDeploymentsCollection(app appWithDao) error {
	collection := &models.Collection{
		Name: "awsDeployments",
		Type: models.CollectionTypeBase,
		Schema: schema.NewSchema(
			&schema.SchemaField{Name: "name", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "user", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "status", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "region", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "costAlertThreshold", Type: schema.FieldTypeNumber},
		),
	}
	return app.Dao().SaveCollection(collection)
}

// bootstrapCostAlertsCollection creates the costAlerts collection.
func bootstrapCostAlertsCollection(app appWithDao) error {
	collection := &models.Collection{
		Name: "costAlerts",
		Type: models.CollectionTypeBase,
		Schema: schema.NewSchema(
			&schema.SchemaField{Name: "deployment", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "user", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "type", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "threshold", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "triggered", Type: schema.FieldTypeBool},
			&schema.SchemaField{Name: "actualCost", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "estimatedCost", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "variance", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "message", Type: schema.FieldTypeText},
			&schema.SchemaField{Name: "serviceBreakdown", Type: schema.FieldTypeJson},
			&schema.SchemaField{Name: "sentAt", Type: schema.FieldTypeDate},
			&schema.SchemaField{Name: "acknowledged", Type: schema.FieldTypeBool},
			&schema.SchemaField{Name: "acknowledgedAt", Type: schema.FieldTypeDate},
		),
	}
	return app.Dao().SaveCollection(collection)
}

// bootstrapCostEstimatesCollection creates the costEstimates collection.
func bootstrapCostEstimatesCollection(app appWithDao) error {
	collection := &models.Collection{
		Name: "costEstimates",
		Type: models.CollectionTypeBase,
		Schema: schema.NewSchema(
			&schema.SchemaField{Name: "deployment", Type: schema.FieldTypeText, Required: true},
			&schema.SchemaField{Name: "totalEstimate", Type: schema.FieldTypeNumber},
			&schema.SchemaField{Name: "breakdown", Type: schema.FieldTypeJson},
		),
	}
	return app.Dao().SaveCollection(collection)
}

// bootstrapAlertTestCollections creates all collections needed by cost alert tests.
// Tests that create deployments and alerts should call this at the start.
func bootstrapAlertTestCollections(app appWithDao) error {
	if err := bootstrapDeploymentsCollection(app); err != nil {
		return err
	}
	if err := bootstrapAwsDeploymentsCollection(app); err != nil {
		return err
	}
	if err := bootstrapCostAlertsCollection(app); err != nil {
		return err
	}
	return bootstrapCostEstimatesCollection(app)
}
