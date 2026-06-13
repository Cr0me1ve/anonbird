package store

import "github.com/netbirdio/netbird/management/server/testutil"

func init() {
	createPostgresTestContainer = testutil.CreatePostgresTestContainer
	createMysqlTestContainer = testutil.CreateMysqlTestContainer
}
