package bookmarks;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

public final class Store {
    private static final String COLS = "id, url, title, description, created_at";

    private static Bookmark scan(ResultSet rs) throws SQLException {
        return new Bookmark(
            rs.getLong("id"),
            rs.getString("url"),
            rs.getString("title"),
            rs.getString("description"),
            rs.getString("created_at"));
    }

    public List<Bookmark> listBookmarks() throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("SELECT " + COLS + " FROM bookmarks ORDER BY id");
             ResultSet rs = ps.executeQuery()) {
            List<Bookmark> bookmarks = new ArrayList<>();
            while (rs.next()) {
                bookmarks.add(scan(rs));
            }
            return bookmarks;
        }
    }

    public Optional<Bookmark> getBookmark(long id) throws SQLException {
        try (Connection conn = Db.getConn()) {
            return getBookmark(conn, id);
        }
    }

    private Optional<Bookmark> getBookmark(Connection conn, long id) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement("SELECT " + COLS + " FROM bookmarks WHERE id = ?")) {
            ps.setLong(1, id);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(scan(rs)) : Optional.empty();
            }
        }
    }

    public Bookmark createBookmark(String url, String title, String description) throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement(
                 "INSERT INTO bookmarks (url, title, description) VALUES (?, ?, ?)",
                 Statement.RETURN_GENERATED_KEYS)) {
            ps.setString(1, url);
            ps.setString(2, title);
            ps.setString(3, description);
            ps.executeUpdate();
            try (ResultSet keys = ps.getGeneratedKeys()) {
                keys.next();
                return getBookmark(conn, keys.getLong(1)).orElseThrow();
            }
        }
    }

    public Optional<Bookmark> updateBookmark(long id, String url, String title, String description)
            throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement(
                 "UPDATE bookmarks SET url = ?, title = ?, description = ? WHERE id = ?")) {
            ps.setString(1, url);
            ps.setString(2, title);
            ps.setString(3, description);
            ps.setLong(4, id);
            if (ps.executeUpdate() == 0) {
                return Optional.empty();
            }
            return getBookmark(conn, id);
        }
    }

    public boolean deleteBookmark(long id) throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("DELETE FROM bookmarks WHERE id = ?")) {
            ps.setLong(1, id);
            return ps.executeUpdate() > 0;
        }
    }
}
