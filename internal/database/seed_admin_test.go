package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"app/internal/config"
)

func TestSeed_CustomAdminAlsoSeedsDefaultAdmin(t *testing.T) {
	db, err := ConnectSQLite()
	require.NoError(t, err)
	require.NoError(t, Migrate(db))

	cfg := &config.Config{AdminUsername: "boss", AdminPassword: "secret"}
	require.NoError(t, Seed(db, cfg))

	var def User
	require.NoError(t, db.Where("username = ?", "admin").First(&def).Error)
	assert.Equal(t, RoleAdmin, def.Role)

	var custom User
	require.NoError(t, db.Where("username = ?", "boss").First(&custom).Error)
	assert.Equal(t, RoleAdmin, custom.Role)
}