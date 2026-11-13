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
    private static final String COLLECTION_COLS = "id, name, created_at";

    /** Result of a collection-membership operation; Routes maps non-OK values to HTTP errors. */
    public enum Outcome { OK, COLLECTION_NOT_FOUND, BOOKMARK_NOT_FOUND, ALREADY_MEMBER, BAD_POSITION, NOT_MEMBER }

    /** Outcome of adding a bookmark; position is the slot it was placed at (meaningful only when OK). */
    public record Placement(Outcome outcome, long position) {}

    /** A unit of work that runs inside a single transaction. */
    private interface Work<T> {
        T run(Connection conn) throws SQLException;
    }

    private static Bookmark scan(ResultSet rs) throws SQLException {
        return new Bookmark(
            rs.getLong("id"),
            rs.getString("url"),
            rs.getString("title"),
            rs.getString("description"),
            rs.getString("created_at"));
    }

    private static Collection scanCollection(ResultSet rs) throws SQLException {
        return new Collection(rs.getLong("id"), rs.getString("name"), rs.getString("created_at"));
    }

    private static <T> T transact(Work<T> work) throws SQLException {
        try (Connection conn = Db.getConn()) {
            conn.setAutoCommit(false);
            try {
                T result = work.run(conn);
                conn.commit();
                return result;
            } catch (SQLException | RuntimeException e) {
                conn.rollback();
                throw e;
            }
        }
    }

    /** Runs a write statement with positional args and returns the affected row count. */
    private static int update(Connection conn, String sql, Object... args) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            for (int i = 0; i < args.length; i++) {
                ps.setObject(i + 1, args[i]);
            }
            return ps.executeUpdate();
        }
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

    /**
     * Deletes the bookmark and its collection memberships (via ON DELETE CASCADE), first closing
     * the position gap it leaves in each collection it belonged to.
     */
    public boolean deleteBookmark(long id) throws SQLException {
        return transact(conn -> {
            List<long[]> memberships = new ArrayList<>();
            try (PreparedStatement ps = conn.prepareStatement(
                     "SELECT collection_id, position FROM collection_bookmarks WHERE bookmark_id = ?")) {
                ps.setLong(1, id);
                try (ResultSet rs = ps.executeQuery()) {
                    while (rs.next()) {
                        memberships.add(new long[] {rs.getLong("collection_id"), rs.getLong("position")});
                    }
                }
            }
            for (long[] m : memberships) {
                update(conn,
                    "UPDATE collection_bookmarks SET position = position - 1 WHERE collection_id = ? AND position > ?",
                    m[0], m[1]);
            }
            return update(conn, "DELETE FROM bookmarks WHERE id = ?", id) > 0;
        });
    }

    // ---- Collections: CRUD ----

    public List<Collection> listCollections() throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("SELECT " + COLLECTION_COLS + " FROM collections ORDER BY id");
             ResultSet rs = ps.executeQuery()) {
            List<Collection> collections = new ArrayList<>();
            while (rs.next()) {
                collections.add(scanCollection(rs));
            }
            return collections;
        }
    }

    public Optional<Collection> getCollection(long id) throws SQLException {
        try (Connection conn = Db.getConn()) {
            return getCollection(conn, id);
        }
    }

    private Optional<Collection> getCollection(Connection conn, long id) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                 "SELECT " + COLLECTION_COLS + " FROM collections WHERE id = ?")) {
            ps.setLong(1, id);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(scanCollection(rs)) : Optional.empty();
            }
        }
    }

    public Collection createCollection(String name) throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement(
                 "INSERT INTO collections (name) VALUES (?)", Statement.RETURN_GENERATED_KEYS)) {
            ps.setString(1, name);
            ps.executeUpdate();
            try (ResultSet keys = ps.getGeneratedKeys()) {
                keys.next();
                return getCollection(conn, keys.getLong(1)).orElseThrow();
            }
        }
    }

    public Optional<Collection> updateCollection(long id, String name) throws SQLException {
        try (Connection conn = Db.getConn()) {
            if (update(conn, "UPDATE collections SET name = ? WHERE id = ?", name, id) == 0) {
                return Optional.empty();
            }
            return getCollection(conn, id);
        }
    }

    /** Deletes the collection; its memberships are removed by cascade, the bookmarks themselves are kept. */
    public boolean deleteCollection(long id) throws SQLException {
        try (Connection conn = Db.getConn()) {
            return update(conn, "DELETE FROM collections WHERE id = ?", id) > 0;
        }
    }

    /** Bookmarks in a collection in position order, or empty if the collection does not exist. */
    public Optional<List<Bookmark>> listCollectionBookmarks(long collectionId) throws SQLException {
        try (Connection conn = Db.getConn()) {
            if (getCollection(conn, collectionId).isEmpty()) {
                return Optional.empty();
            }
            try (PreparedStatement ps = conn.prepareStatement(
                     "SELECT b.id, b.url, b.title, b.description, b.created_at "
                     + "FROM collection_bookmarks cb JOIN bookmarks b ON b.id = cb.bookmark_id "
                     + "WHERE cb.collection_id = ? ORDER BY cb.position")) {
                ps.setLong(1, collectionId);
                try (ResultSet rs = ps.executeQuery()) {
                    List<Bookmark> bookmarks = new ArrayList<>();
                    while (rs.next()) {
                        bookmarks.add(scan(rs));
                    }
                    return Optional.of(bookmarks);
                }
            }
        }
    }

    // ---- Collections: membership and ordering ----

    /**
     * Adds a bookmark to a collection at {@code position} (0..count), or appends when position is null.
     * Bookmarks at or after the insertion point shift up by one.
     */
    public Placement addBookmarkToCollection(long collectionId, long bookmarkId, Long position) throws SQLException {
        return transact(conn -> {
            if (getCollection(conn, collectionId).isEmpty()) {
                return new Placement(Outcome.COLLECTION_NOT_FOUND, 0);
            }
            if (getBookmark(conn, bookmarkId).isEmpty()) {
                return new Placement(Outcome.BOOKMARK_NOT_FOUND, 0);
            }
            if (positionOf(conn, collectionId, bookmarkId).isPresent()) {
                return new Placement(Outcome.ALREADY_MEMBER, 0);
            }
            long count = countIn(conn, collectionId);
            long pos = position == null ? count : position;
            if (pos < 0 || pos > count) {
                return new Placement(Outcome.BAD_POSITION, 0);
            }
            update(conn,
                "UPDATE collection_bookmarks SET position = position + 1 WHERE collection_id = ? AND position >= ?",
                collectionId, pos);
            update(conn,
                "INSERT INTO collection_bookmarks (collection_id, bookmark_id, position) VALUES (?, ?, ?)",
                collectionId, bookmarkId, pos);
            return new Placement(Outcome.OK, pos);
        });
    }

    /** Removes a bookmark from a collection and shifts higher positions down by one. */
    public Outcome removeBookmarkFromCollection(long collectionId, long bookmarkId) throws SQLException {
        return transact(conn -> {
            if (getCollection(conn, collectionId).isEmpty()) {
                return Outcome.COLLECTION_NOT_FOUND;
            }
            Optional<Long> pos = positionOf(conn, collectionId, bookmarkId);
            if (pos.isEmpty()) {
                return Outcome.NOT_MEMBER;
            }
            update(conn, "DELETE FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?",
                collectionId, bookmarkId);
            update(conn,
                "UPDATE collection_bookmarks SET position = position - 1 WHERE collection_id = ? AND position > ?",
                collectionId, pos.get());
            return Outcome.OK;
        });
    }

    /** Moves a member bookmark to {@code newPosition} (0..count-1), shifting the bookmarks in between. */
    public Outcome moveBookmark(long collectionId, long bookmarkId, long newPosition) throws SQLException {
        return transact(conn -> {
            if (getCollection(conn, collectionId).isEmpty()) {
                return Outcome.COLLECTION_NOT_FOUND;
            }
            Optional<Long> from = positionOf(conn, collectionId, bookmarkId);
            if (from.isEmpty()) {
                return Outcome.NOT_MEMBER;
            }
            long count = countIn(conn, collectionId);
            if (newPosition < 0 || newPosition >= count) {
                return Outcome.BAD_POSITION;
            }
            long old = from.get();
            if (newPosition > old) {
                update(conn,
                    "UPDATE collection_bookmarks SET position = position - 1 "
                    + "WHERE collection_id = ? AND position > ? AND position <= ?",
                    collectionId, old, newPosition);
            } else if (newPosition < old) {
                update(conn,
                    "UPDATE collection_bookmarks SET position = position + 1 "
                    + "WHERE collection_id = ? AND position >= ? AND position < ?",
                    collectionId, newPosition, old);
            }
            update(conn, "UPDATE collection_bookmarks SET position = ? WHERE collection_id = ? AND bookmark_id = ?",
                newPosition, collectionId, bookmarkId);
            return Outcome.OK;
        });
    }

    private static Optional<Long> positionOf(Connection conn, long collectionId, long bookmarkId) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                 "SELECT position FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?")) {
            ps.setLong(1, collectionId);
            ps.setLong(2, bookmarkId);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(rs.getLong("position")) : Optional.empty();
            }
        }
    }

    private static long countIn(Connection conn, long collectionId) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                 "SELECT COUNT(*) FROM collection_bookmarks WHERE collection_id = ?")) {
            ps.setLong(1, collectionId);
            try (ResultSet rs = ps.executeQuery()) {
                rs.next();
                return rs.getLong(1);
            }
        }
    }
}
