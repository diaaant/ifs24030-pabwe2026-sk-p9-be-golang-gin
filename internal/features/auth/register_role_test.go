package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"app/internal/database"
)

func TestRegister_RoleSelection(t *testing.T) {
	db, err := database.ConnectSQLite()
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	svc := NewService(NewRepository(db), "secret")

	admin, err := svc.Register(RegisterRequest{
		Username: "adminbaru", Email: "adminbaru@x.com", Password: "123456", Role: "admin",
	})
	require.NoError(t, err)
	assert.Equal(t, database.RoleAdmin, admin.User.Role)

	member, err := svc.Register(RegisterRequest{
		Username: "memberbaru", Email: "memberbaru@x.com", Password: "123456",
	})
	require.NoError(t, err)
	assert.Equal(t, database.RoleMember, member.User.Role)
}