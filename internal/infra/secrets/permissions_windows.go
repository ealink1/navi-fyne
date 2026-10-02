package secrets

import "os"

// Windows access is inherited from the user's application-data directory ACL;
// Unix permission bits do not represent that ACL.
func privatePermissions(os.FileInfo) bool { return true }

// Directory fsync is not supported by os.File on Windows; files are synced
// before publishing. Native Windows durability and ACL behavior need acceptance.
func syncDirectory(string) error { return nil }
