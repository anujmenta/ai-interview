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

    // create new bookmark
    private static Bookmark scan(ResultSet rs) throws SQLException {
        return new Bookmark(
            rs.getLong("id"),
            rs.getString("url"),
            rs.getString("title"),
            rs.getString("description"),
            rs.getString("created_at"));
    }

    // list all bookmarks
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

    // get bookmark by id
    public Optional<Bookmark> getBookmark(long id) throws SQLException {
        try (Connection conn = Db.getConn()) {
            return getBookmark(conn, id);
        }
    }

    // get bookmark by id  - overload method
    private Optional<Bookmark> getBookmark(Connection conn, long id) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement("SELECT " + COLS + " FROM bookmarks WHERE id = ?")) {
            ps.setLong(1, id);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? Optional.of(scan(rs)) : Optional.empty();
            }
        }
    }

    // creating new bookmark
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

    // update a bookmark
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

    // delete bookmark
    public boolean deleteBookmark(long id) throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("DELETE FROM bookmarks WHERE id = ?")) {
            ps.setLong(1, id);
            return ps.executeUpdate() > 0;
        }
    }

    // ----- Collections -----

    private static final String COLLECTION_COLS = "id, name, created_at";

    private static Collection scanCollection(ResultSet rs) throws SQLException {
        return new Collection(rs.getLong("id"), rs.getString("name"), rs.getString("created_at"));
    }

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
        try (PreparedStatement ps = conn.prepareStatement("SELECT " + COLLECTION_COLS + " FROM collections WHERE id = ?")) {
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
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("UPDATE collections SET name = ? WHERE id = ?")) {
            ps.setString(1, name);
            ps.setLong(2, id);
            if (ps.executeUpdate() == 0) {
                return Optional.empty();
            }
            return getCollection(conn, id);
        }
    }

    public boolean deleteCollection(long id) throws SQLException {
        try (Connection conn = Db.getConn();
             PreparedStatement ps = conn.prepareStatement("DELETE FROM collections WHERE id = ?")) {
            ps.setLong(1, id);
            return ps.executeUpdate() > 0;
        }
    }

    // ----- Collection membership & ordering -----

    // list a collection's bookmarks, ordered by the user's chosen position
    public List<Bookmark> listCollectionBookmarks(long collectionId) throws SQLException {
        try (Connection conn = Db.getConn()) {
            return listCollectionBookmarks(conn, collectionId);
        }
    }

    private List<Bookmark> listCollectionBookmarks(Connection conn, long collectionId) throws SQLException {
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
                return bookmarks;
            }
        }
    }

    // add a bookmark to a collection, appended after the current last position;
    // empty result means the bookmark is already a member of the collection
    public Optional<CollectionBookmark> addBookmarkToCollection(long collectionId, long bookmarkId) throws SQLException {
        try (Connection conn = Db.getConn()) {
            if (isMember(conn, collectionId, bookmarkId)) {
                return Optional.empty();
            }
            int position;
            try (PreparedStatement next = conn.prepareStatement(
                    "SELECT COALESCE(MAX(position), -1) + 1 FROM collection_bookmarks WHERE collection_id = ?")) {
                next.setLong(1, collectionId);
                try (ResultSet rs = next.executeQuery()) {
                    rs.next();
                    position = rs.getInt(1);
                }
            }
            try (PreparedStatement insert = conn.prepareStatement(
                    "INSERT INTO collection_bookmarks (collection_id, bookmark_id, position) VALUES (?, ?, ?)")) {
                insert.setLong(1, collectionId);
                insert.setLong(2, bookmarkId);
                insert.setInt(3, position);
                insert.executeUpdate();
            }
            Bookmark bookmark = getBookmark(conn, bookmarkId).orElseThrow();
            return Optional.of(new CollectionBookmark(bookmark, position));
        }
    }

    // remove a bookmark from a collection and compact the gap left behind
    public boolean removeBookmarkFromCollection(long collectionId, long bookmarkId) throws SQLException {
        try (Connection conn = Db.getConn()) {
            Integer position = currentPosition(conn, collectionId, bookmarkId);
            if (position == null) {
                return false;
            }
            try (PreparedStatement delete = conn.prepareStatement(
                    "DELETE FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?")) {
                delete.setLong(1, collectionId);
                delete.setLong(2, bookmarkId);
                delete.executeUpdate();
            }
            shiftPositions(conn, collectionId, position + 1, Integer.MAX_VALUE, -1);
            return true;
        }
    }

    // move a bookmark to a new (0-based) position within a collection, shifting
    // the bookmarks between the old and new position to keep positions dense
    public MoveResult moveBookmark(long collectionId, long bookmarkId, int newPosition) throws SQLException {
        try (Connection conn = Db.getConn()) {
            Integer currentPosition = currentPosition(conn, collectionId, bookmarkId);
            if (currentPosition == null) {
                return new MoveResult(MoveResult.Status.NOT_MEMBER, null);
            }
            int count = countCollectionBookmarks(conn, collectionId);
            if (newPosition < 0 || newPosition >= count) {
                return new MoveResult(MoveResult.Status.OUT_OF_RANGE, null);
            }
            if (newPosition < currentPosition) {
                shiftPositions(conn, collectionId, newPosition, currentPosition - 1, 1);
            } else if (newPosition > currentPosition) {
                shiftPositions(conn, collectionId, currentPosition + 1, newPosition, -1);
            }
            try (PreparedStatement setPosition = conn.prepareStatement(
                    "UPDATE collection_bookmarks SET position = ? WHERE collection_id = ? AND bookmark_id = ?")) {
                setPosition.setInt(1, newPosition);
                setPosition.setLong(2, collectionId);
                setPosition.setLong(3, bookmarkId);
                setPosition.executeUpdate();
            }
            return new MoveResult(MoveResult.Status.OK, listCollectionBookmarks(conn, collectionId));
        }
    }

    private boolean isMember(Connection conn, long collectionId, long bookmarkId) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                "SELECT 1 FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?")) {
            ps.setLong(1, collectionId);
            ps.setLong(2, bookmarkId);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next();
            }
        }
    }

    private Integer currentPosition(Connection conn, long collectionId, long bookmarkId) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                "SELECT position FROM collection_bookmarks WHERE collection_id = ? AND bookmark_id = ?")) {
            ps.setLong(1, collectionId);
            ps.setLong(2, bookmarkId);
            try (ResultSet rs = ps.executeQuery()) {
                return rs.next() ? rs.getInt(1) : null;
            }
        }
    }

    private int countCollectionBookmarks(Connection conn, long collectionId) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                "SELECT COUNT(*) FROM collection_bookmarks WHERE collection_id = ?")) {
            ps.setLong(1, collectionId);
            try (ResultSet rs = ps.executeQuery()) {
                rs.next();
                return rs.getInt(1);
            }
        }
    }

    // shift positions in [fromPos, toPos] by delta; used both to close the gap
    // left by a removal and to make room for a move
    private void shiftPositions(Connection conn, long collectionId, int fromPos, int toPos, int delta) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                "UPDATE collection_bookmarks SET position = position + ? "
                    + "WHERE collection_id = ? AND position BETWEEN ? AND ?")) {
            ps.setInt(1, delta);
            ps.setLong(2, collectionId);
            ps.setInt(3, fromPos);
            ps.setInt(4, toPos);
            ps.executeUpdate();
        }
    }
}
