package bookmarks;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;
import org.sqlite.SQLiteConfig;

public final class Db {
    private static final String PATH = System.getenv().getOrDefault("DB_PATH", "bookmarks.db");

    // one entry per DDL statement: the xerial sqlite-jdbc driver's
    // Statement.execute() only runs the first statement in a ";"-separated
    // string, so each CREATE TABLE must be executed individually
    private static final String[] SCHEMA = {
        """
        CREATE TABLE IF NOT EXISTS bookmarks (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            url TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )""",
        """
        CREATE TABLE IF NOT EXISTS collections (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )""",
        """
        CREATE TABLE IF NOT EXISTS collection_bookmarks (
            collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
            bookmark_id INTEGER NOT NULL REFERENCES bookmarks(id) ON DELETE CASCADE,
            position INTEGER NOT NULL,
            PRIMARY KEY (collection_id, bookmark_id)
        )""",
    };

    private Db() {}

    public static Connection getConn() throws SQLException {
        SQLiteConfig config = new SQLiteConfig();
        config.enforceForeignKeys(true);
        return DriverManager.getConnection("jdbc:sqlite:" + PATH, config.toProperties());
    }

    public static void init() throws SQLException {
        try (Connection conn = getConn(); Statement stmt = conn.createStatement()) {
            for (String ddl : SCHEMA) {
                stmt.execute(ddl);
            }
        }
    }
}
