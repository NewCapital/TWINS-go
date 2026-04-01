#include <stdlib.h>
#include <string.h>
#include <db.h>

// Helper function to open BerkeleyDB
int bdb_wrapper_open(DB **dbp, const char *file) {
    int ret;

    ret = db_create(dbp, NULL, 0);
    if (ret != 0) {
        return ret;
    }

    // Open the database - DB_RDONLY for read-only access
    // Use DB_UNKNOWN to automatically detect database type (BTREE, HASH, etc.)
    // Pass "main" as database name which is common in Bitcoin/TWINS wallets
    ret = (*dbp)->open(*dbp, NULL, file, "main", DB_UNKNOWN, DB_RDONLY, 0);
    return ret;
}

// Helper function to close BerkeleyDB
int bdb_wrapper_close(DB *dbp) {
    if (dbp != NULL) {
        return dbp->close(dbp, 0);
    }
    return 0;
}

// Helper to iterate through all entries
typedef struct {
    void *key_data;
    size_t key_size;
    void *val_data;
    size_t val_size;
    int ret;
} entry_result;

// Get next entry from cursor
int bdb_cursor_next(DBC *cursor, entry_result *result) {
    DBT key, data;
    memset(&key, 0, sizeof(DBT));
    memset(&data, 0, sizeof(DBT));

    int ret = cursor->get(cursor, &key, &data, DB_NEXT);

    if (ret == 0) {
        result->key_data = key.data;
        result->key_size = key.size;
        result->val_data = data.data;
        result->val_size = data.size;
    }
    result->ret = ret;

    return ret;
}

// Create a cursor
int bdb_create_cursor(DB *dbp, DBC **cursor) {
    return dbp->cursor(dbp, NULL, cursor, 0);
}

// Close a cursor
int bdb_close_cursor(DBC *cursor) {
    if (cursor != NULL) {
        return cursor->close(cursor);
    }
    return 0;
}