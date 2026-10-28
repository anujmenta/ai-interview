package bookmarks;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.sql.Statement;
import org.sqlite.SQLiteConfig;

public final class Db {
    private static final String PATH = System.getenv().getOrDefault("DB_PATH", "bookmarks.db");

    private static final String SCHEMA = """
        CREATE TABLE IF NOT EXISTS bookmarks (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            url TEXT NOT NULL,
            title TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
        )""";

    private Db() {}

    public static Connection getConn() throws SQLException {
        SQLiteConfig config = new SQLiteConfig();
        config.enforceForeignKeys(true);
        return DriverManager.getConnection("jdbc:sqlite:" + PATH, config.toProperties());
    }

    public static void init() throws SQLException {
        try (Connection conn = getConn(); Statement stmt = conn.createStatement()) {
            stmt.execute(SCHEMA);
        }
    }
}
