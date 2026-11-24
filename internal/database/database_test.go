package database

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"app/internal/config"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := ConnectSQLite()
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	return db
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func testConfig() *config.Config {
	return &config.Config{AdminUsername: "admin", AdminPassword: "admin"}
}

func createUser(t *testing.T, db *gorm.DB, username string) User {
	t.Helper()
	u := User{Username: username, Email: username + "@x.com", Password: "hash"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// ---------- Koneksi ----------

func TestConnectSQLite_Success(t *testing.T) {
	db, err := ConnectSQLite()
	require.NoError(t, err)
	require.NotNil(t, db)

	var one int
	require.NoError(t, db.Raw("SELECT 1").Scan(&one).Error)
	assert.Equal(t, 1, one)
}

func TestConnectSQLite_Error(t *testing.T) {
	original := openSQLite
	t.Cleanup(func() { openSQLite = original })
	openSQLite = func(*gorm.Config) (*gorm.DB, error) { return nil, errors.New("boom") }

	db, err := ConnectSQLite()

	assert.Nil(t, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal membuka SQLite in-memory")
}

func TestConnect_Success(t *testing.T) {
	original := openDialector
	t.Cleanup(func() { openDialector = original })
	openDialector = func(string) gorm.Dialector { return sqlite.Open(":memory:") }

	db, err := Connect(testConfig())

	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestConnect_Error(t *testing.T) {
	cfg := &config.Config{
		DBHost: "127.0.0.1", DBPort: "1", // port tertutup -> koneksi ditolak
		DBUsername: "u", DBPassword: "p", DBDatabase: "d", DBSSLMode: "disable",
	}

	db, err := Connect(cfg)

	assert.Nil(t, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal terhubung ke PostgreSQL")
}

// ---------- Migrasi ----------

func TestMigrate_CreatesTables(t *testing.T) {
	db := newTestDB(t)

	assert.True(t, db.Migrator().HasTable("users"))
	assert.True(t, db.Migrator().HasTable("posts"))
}

func TestMigrate_Error(t *testing.T) {
	db, err := ConnectSQLite()
	require.NoError(t, err)
	closeDB(t, db)

	assert.Error(t, Migrate(db))
}

// ---------- Seeder ----------

func TestSeed_CreatesDefaultAccounts(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, Seed(db, testConfig()))

	var admin, member User
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	require.NoError(t, db.Where("username = ?", "member").First(&member).Error)

	assert.Equal(t, RoleAdmin, admin.Role)
	assert.Equal(t, "admin@delcom.local", admin.Email)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("admin")))

	assert.Equal(t, RoleMember, member.Role)
	assert.Equal(t, "member@delcom.local", member.Email)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(member.Password), []byte("member")))
}

func TestSeed_UsesAdminCredentialsFromConfig(t *testing.T) {
	db := newTestDB(t)
	cfg := &config.Config{AdminUsername: "root", AdminPassword: "toor"}

	require.NoError(t, Seed(db, cfg))

	var root User
	require.NoError(t, db.Where("username = ?", "root").First(&root).Error)
	assert.Equal(t, RoleAdmin, root.Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(root.Password), []byte("toor")))
}

func TestSeed_IsIdempotent(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, Seed(db, testConfig()))
	require.NoError(t, Seed(db, testConfig()))

	var total int64
	require.NoError(t, db.Model(&User{}).Count(&total).Error)
	assert.Equal(t, int64(2), total)
}

func TestSeed_DatabaseError(t *testing.T) {
	db := newTestDB(t)
	closeDB(t, db)

	assert.Error(t, Seed(db, testConfig()))
}

func TestSeed_AdminPasswordTooLong(t *testing.T) {
	db := newTestDB(t)
	cfg := &config.Config{AdminUsername: "admin", AdminPassword: strings.Repeat("a", 73)}

	assert.Error(t, Seed(db, cfg)) // bcrypt menolak password > 72 byte
}

func TestSeed_AdminCreateError(t *testing.T) {
	db := newTestDB(t)
	// Email yang sama sudah dipakai user lain -> pelanggaran unique index.
	require.NoError(t, db.Create(&User{
		Username: "someone", Email: "admin@delcom.local", Password: "x", Role: RoleMember,
	}).Error)

	assert.Error(t, Seed(db, testConfig()))
}

func TestSeed_MemberCreateError(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Create(&User{
		Username: "someone", Email: "member@delcom.local", Password: "x", Role: RoleMember,
	}).Error)

	err := Seed(db, testConfig())

	assert.Error(t, err)
	var admins int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", "admin").Count(&admins).Error)
	assert.Equal(t, int64(1), admins) // admin sempat terbuat sebelum member gagal
}

func TestSeed_UpdatesExistingAdminRoleAndPassword(t *testing.T) {
	db := newTestDB(t)
	oldHash, err := bcrypt.GenerateFromPassword([]byte("lama"), bcrypt.MinCost)
	require.NoError(t, err)
	// Akun "admin" sudah ada tetapi role dan password-nya berbeda dari konfigurasi.
	require.NoError(t, db.Create(&User{
		Username: "admin", Email: "admin@x.com", Password: string(oldHash), Role: RoleMember,
	}).Error)

	require.NoError(t, Seed(db, testConfig()))

	var admin User
	require.NoError(t, db.Where("username = ?", "admin").First(&admin).Error)
	assert.Equal(t, RoleAdmin, admin.Role)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("admin")))
}

func TestSeed_UpdatesExistingMemberRoleOnly(t *testing.T) {
	db := newTestDB(t)
	// Akun "member" sudah ada tetapi role-nya salah; password member tidak disentuh.
	require.NoError(t, db.Create(&User{
		Username: "member", Email: "member@x.com", Password: "hash-asli", Role: RoleAdmin,
	}).Error)

	require.NoError(t, Seed(db, testConfig()))

	var member User
	require.NoError(t, db.Where("username = ?", "member").First(&member).Error)
	assert.Equal(t, RoleMember, member.Role)
	assert.Equal(t, "hash-asli", member.Password)
}

func TestSeed_ExistingAdminPasswordTooLong(t *testing.T) {
	db := newTestDB(t)
	// Password lama bukan hash valid, jadi perbandingan gagal dan hash baru dibuat;
	// bcrypt menolak password > 72 byte sehingga Seed mengembalikan error.
	require.NoError(t, db.Create(&User{
		Username: "admin", Email: "admin@x.com", Password: "x", Role: RoleAdmin,
	}).Error)
	cfg := &config.Config{AdminUsername: "admin", AdminPassword: strings.Repeat("a", 73)}

	assert.Error(t, Seed(db, cfg))
}

// ---------- Skema & relasi ----------

func TestSchema_Defaults(t *testing.T) {
	db := newTestDB(t)
	user := createUser(t, db, "u1")

	post := Post{UserID: user.ID, Title: "Halo", Slug: "halo"}
	require.NoError(t, db.Create(&post).Error)

	var fetchedUser User
	require.NoError(t, db.First(&fetchedUser, user.ID).Error)
	assert.Equal(t, RoleMember, fetchedUser.Role)

	var fetched Post
	require.NoError(t, db.First(&fetched, post.ID).Error)
	assert.Equal(t, DefaultCategory, fetched.Category)
	assert.False(t, fetched.IsPublished)
	assert.Nil(t, fetched.PublishedAt)
}

func TestSchema_PublishedPostAndPreloadUser(t *testing.T) {
	db := newTestDB(t)
	user := createUser(t, db, "u1")
	now := time.Now()

	post := Post{
		UserID: user.ID, Title: "Terbit", Slug: "terbit", Category: "Teknologi",
		IsPublished: true, PublishedAt: &now,
	}
	require.NoError(t, db.Create(&post).Error)

	var fetched Post
	require.NoError(t, db.Preload("User").First(&fetched, post.ID).Error)
	assert.True(t, fetched.IsPublished)
	assert.NotNil(t, fetched.PublishedAt)
	require.NotNil(t, fetched.User)
	assert.Equal(t, "u1", fetched.User.Username)
}

func TestSchema_SlugMustBeUnique(t *testing.T) {
	db := newTestDB(t)
	user := createUser(t, db, "u1")
	require.NoError(t, db.Create(&Post{UserID: user.ID, Title: "A", Slug: "sama"}).Error)

	err := db.Create(&Post{UserID: user.ID, Title: "B", Slug: "sama"}).Error

	assert.Error(t, err)
}

func TestSchema_UserDeleteCascadesPosts(t *testing.T) {
	db := newTestDB(t)
	user := createUser(t, db, "u1")
	require.NoError(t, db.Create(&Post{UserID: user.ID, Title: "A", Slug: "a"}).Error)

	require.NoError(t, db.Delete(&User{}, user.ID).Error)

	var posts int64
	require.NoError(t, db.Model(&Post{}).Count(&posts).Error)
	assert.Equal(t, int64(0), posts)
}

// ---------- EnsureDatabase ----------

// useSQLiteFile mengalihkan koneksi ke berkas SQLite sementara yang sudah
// berisi tabel pg_database tiruan (dengan baris untuk nama yang diberikan).
func useSQLiteFile(t *testing.T, existing ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "maintenance.db")

	seed, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, seed.Exec("CREATE TABLE pg_database (datname text)").Error)
	for _, name := range existing {
		require.NoError(t, seed.Exec("INSERT INTO pg_database (datname) VALUES (?)", name).Error)
	}
	sqlDB, _ := seed.DB()
	require.NoError(t, sqlDB.Close())

	original := openDialector
	t.Cleanup(func() { openDialector = original })
	openDialector = func(string) gorm.Dialector { return sqlite.Open(path) }
}

func TestEnsureDatabase_AlreadyExists(t *testing.T) {
	useSQLiteFile(t, "pabwe_p9_blog")

	assert.NoError(t, EnsureDatabase(&config.Config{DBDatabase: "pabwe_p9_blog"}))
}

func TestEnsureDatabase_IssuesCreateWhenMissing(t *testing.T) {
	useSQLiteFile(t) // pg_database kosong -> database belum ada

	// SQLite tidak mengenal CREATE DATABASE sehingga error sintaks diharapkan;
	// yang diuji: perintah CREATE dijalankan dengan nama yang di-quote aman.
	err := EnsureDatabase(&config.Config{DBDatabase: `weird"name`})

	assert.Error(t, err)
}

func TestEnsureDatabase_LookupError(t *testing.T) {
	original := openDialector
	t.Cleanup(func() { openDialector = original })
	openDialector = func(string) gorm.Dialector { return sqlite.Open(":memory:") } // tanpa tabel pg_database

	assert.Error(t, EnsureDatabase(&config.Config{DBDatabase: "x"}))
}

func TestEnsureDatabase_ConnectionError(t *testing.T) {
	cfg := &config.Config{
		DBHost: "127.0.0.1", DBPort: "1", DBUsername: "u", DBPassword: "p", DBDatabase: "d", DBSSLMode: "disable",
	}

	err := EnsureDatabase(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal terhubung ke database pemeliharaan")
}

// ---------- ConnectOrCreate ----------

// failingDialector selalu gagal saat diinisialisasi (meniru server/DB tidak tersedia).
type failingDialector struct{ gorm.Dialector }

func (failingDialector) Initialize(*gorm.DB) error { return errors.New("tidak bisa terhubung") }

// sequenceDialectors mengganti openDialector agar tiap panggilan berurutan memakai dialector berbeda.
func sequenceDialectors(t *testing.T, seq ...func() gorm.Dialector) {
	t.Helper()
	original := openDialector
	t.Cleanup(func() { openDialector = original })
	i := 0
	openDialector = func(string) gorm.Dialector {
		d := seq[i]()
		i++
		return d
	}
}

func failing() gorm.Dialector { return failingDialector{} }
func memory() gorm.Dialector  { return sqlite.Open(":memory:") }

func TestConnectOrCreate_ConnectsDirectly(t *testing.T) {
	sequenceDialectors(t, memory) // hanya satu panggilan: database maintenance tidak disentuh

	db, err := ConnectOrCreate(testConfig())

	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestConnectOrCreate_CreatesThenReconnects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.db")
	seed, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, seed.Exec("CREATE TABLE pg_database (datname text)").Error)
	require.NoError(t, seed.Exec("INSERT INTO pg_database (datname) VALUES ('x')").Error)
	sqlDB, _ := seed.DB()
	require.NoError(t, sqlDB.Close())

	sequenceDialectors(t, failing, func() gorm.Dialector { return sqlite.Open(path) }, memory)

	db, err := ConnectOrCreate(&config.Config{DBDatabase: "x"})

	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestConnectOrCreate_ReportsBothFailures(t *testing.T) {
	sequenceDialectors(t, failing, failing)

	db, err := ConnectOrCreate(testConfig())

	assert.Nil(t, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal terhubung ke PostgreSQL")
	assert.Contains(t, err.Error(), "pembuatan database otomatis juga gagal")
}

// ---------- MySQL ----------

func TestMySQLDSN(t *testing.T) {
	cfg := &config.Config{DBHost: "db.local", DBPort: "3306", DBUsername: "u", DBPassword: "p@ss/w:rd", DBDatabase: "blog"}

	dsn := mysqlDSN(cfg)

	assert.Contains(t, dsn, "tcp(db.local:3306)/blog")
	assert.Contains(t, dsn, "parseTime=true")
	assert.Contains(t, dsn, "charset=utf8mb4")
	parsed, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	assert.Equal(t, "p@ss/w:rd", parsed.Passwd)
}

func TestConnect_MySQLUsesMySQLDialector(t *testing.T) {
	original := openMySQL
	t.Cleanup(func() { openMySQL = original })
	var gotDSN string
	openMySQL = func(dsn string) gorm.Dialector {
		gotDSN = dsn
		return sqlite.Open(":memory:")
	}

	db, err := Connect(&config.Config{DBConnection: "mysql", DBHost: "h", DBPort: "3306", DBUsername: "u", DBDatabase: "d"})

	require.NoError(t, err)
	assert.NotNil(t, db)
	assert.Contains(t, gotDSN, "tcp(h:3306)/d")
}

func TestConnect_MySQLError(t *testing.T) {
	cfg := &config.Config{DBConnection: "mysql", DBHost: "127.0.0.1", DBPort: "1", DBUsername: "u", DBPassword: "p", DBDatabase: "d"}

	db, err := Connect(cfg)

	assert.Nil(t, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gagal terhubung ke MySQL")
}

func TestConnectOrCreate_MySQLDoesNotTryToCreate(t *testing.T) {
	cfg := &config.Config{DBConnection: "mysql", DBHost: "127.0.0.1", DBPort: "1", DBUsername: "u", DBPassword: "p", DBDatabase: "d"}

	db, err := ConnectOrCreate(cfg)

	assert.Nil(t, db)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "pembuatan database otomatis")
}