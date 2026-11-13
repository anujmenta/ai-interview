package bookmarks;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;
import java.sql.SQLException;
import java.util.Map;
import java.util.Optional;

public final class Routes implements HttpHandler {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final String NOT_FOUND = "bookmark not found";
    private static final String COLLECTION_NOT_FOUND = "collection not found";
    private static final String NOT_IN_COLLECTION = "bookmark not in collection";

    private final Store store;

    public Routes(Store store) {
        this.store = store;
    }

    private record Input(String url, String title, String description) {}

    /** Thrown for client errors; carries the status and message returned to the caller. */
    private static final class HttpError extends Exception {
        final int status;

        HttpError(int status, String message) {
            super(message);
            this.status = status;
        }
    }

    @Override
    public void handle(HttpExchange ex) throws IOException {
        try {
            route(ex);
        } catch (HttpError e) {
            writeJson(ex, e.status, Map.of("error", e.getMessage()));
        } catch (SQLException e) {
            writeJson(ex, 500, Map.of("error", String.valueOf(e.getMessage())));
        } finally {
            ex.close();
        }
    }

    private void route(HttpExchange ex) throws IOException, SQLException, HttpError {
        String method = ex.getRequestMethod();
        String path = ex.getRequestURI().getPath();
        if (path.length() > 1 && path.endsWith("/")) {
            path = path.substring(0, path.length() - 1);
        }

        if (path.equals("/bookmarks")) {
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.listBookmarks());
                case "POST" -> {
                    Input in = parseInput(ex);
                    writeJson(ex, 201, store.createBookmark(in.url(), in.title(), in.description()));
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (path.startsWith("/bookmarks/")) {
            long id = parseId(path.substring("/bookmarks/".length()));
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.getBookmark(id).orElseThrow(() -> new HttpError(404, NOT_FOUND)));
                case "PUT" -> {
                    Input in = parseInput(ex);
                    Optional<Bookmark> updated = store.updateBookmark(id, in.url(), in.title(), in.description());
                    writeJson(ex, 200, updated.orElseThrow(() -> new HttpError(404, NOT_FOUND)));
                }
                case "DELETE" -> {
                    if (!store.deleteBookmark(id)) {
                        throw new HttpError(404, NOT_FOUND);
                    }
                    ex.sendResponseHeaders(204, -1);
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (path.equals("/collections") || path.startsWith("/collections/")) {
            routeCollections(ex, method, path);
            return;
        }

        throw new HttpError(404, "not found");
    }

    /**
     * Collection routes:
     *   /collections                                        GET, POST
     *   /collections/{id}                                   GET, PUT, DELETE
     *   /collections/{id}/bookmarks                         GET, POST
     *   /collections/{id}/bookmarks/{bookmarkId}            DELETE
     *   /collections/{id}/bookmarks/{bookmarkId}/position   PUT
     */
    private void routeCollections(HttpExchange ex, String method, String path)
            throws IOException, SQLException, HttpError {
        String rest = path.equals("/collections") ? "" : path.substring("/collections/".length());
        String[] seg = rest.isEmpty() ? new String[0] : rest.split("/", -1);

        if (seg.length == 0) {
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.listCollections());
                case "POST" -> writeJson(ex, 201, store.createCollection(parseName(ex)));
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        long cid = parseId(seg[0], COLLECTION_NOT_FOUND);

        if (seg.length == 1) {
            switch (method) {
                case "GET" -> writeJson(ex, 200,
                    store.getCollection(cid).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                case "PUT" -> {
                    String name = parseName(ex);
                    writeJson(ex, 200,
                        store.updateCollection(cid, name).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                }
                case "DELETE" -> {
                    if (!store.deleteCollection(cid)) {
                        throw new HttpError(404, COLLECTION_NOT_FOUND);
                    }
                    ex.sendResponseHeaders(204, -1);
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (seg.length == 2 && seg[1].equals("bookmarks")) {
            switch (method) {
                case "GET" -> writeJson(ex, 200,
                    store.listCollectionBookmarks(cid).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                case "POST" -> {
                    JsonNode data = parseObject(ex);
                    long bookmarkId = requiredLong(data, "bookmarkId");
                    Long position = optionalLong(data, "position");
                    Store.Placement placed = store.addBookmarkToCollection(cid, bookmarkId, position);
                    checkOutcome(placed.outcome());
                    writeJson(ex, 201, Map.of(
                        "collectionId", cid,
                        "bookmarkId", bookmarkId,
                        "position", placed.position()));
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (seg.length == 3 && seg[1].equals("bookmarks")) {
            long bookmarkId = parseId(seg[2]);
            switch (method) {
                case "DELETE" -> {
                    checkOutcome(store.removeBookmarkFromCollection(cid, bookmarkId));
                    ex.sendResponseHeaders(204, -1);
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (seg.length == 4 && seg[1].equals("bookmarks") && seg[3].equals("position")) {
            long bookmarkId = parseId(seg[2]);
            switch (method) {
                case "PUT" -> {
                    JsonNode data = parseObject(ex);
                    long newPosition = requiredLong(data, "position");
                    checkOutcome(store.moveBookmark(cid, bookmarkId, newPosition));
                    writeJson(ex, 200,
                        store.listCollectionBookmarks(cid).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        throw new HttpError(404, "not found");
    }

    /** Maps a non-OK membership outcome to its HTTP error. */
    private static void checkOutcome(Store.Outcome outcome) throws HttpError {
        switch (outcome) {
            case OK -> { }
            case COLLECTION_NOT_FOUND -> throw new HttpError(404, COLLECTION_NOT_FOUND);
            case BOOKMARK_NOT_FOUND -> throw new HttpError(404, NOT_FOUND);
            case ALREADY_MEMBER -> throw new HttpError(409, "bookmark already in collection");
            case BAD_POSITION -> throw new HttpError(400, "position out of range");
            case NOT_MEMBER -> throw new HttpError(404, NOT_IN_COLLECTION);
        }
    }

    private static long parseId(String raw) throws HttpError {
        return parseId(raw, NOT_FOUND);
    }

    private static long parseId(String raw, String notFoundMessage) throws HttpError {
        try {
            return Long.parseLong(raw);
        } catch (NumberFormatException e) {
            throw new HttpError(404, notFoundMessage);
        }
    }

    private static JsonNode parseObject(HttpExchange ex) throws IOException, HttpError {
        JsonNode data;
        try (InputStream body = ex.getRequestBody()) {
            data = JSON.readTree(body);
        } catch (IOException e) {
            throw new HttpError(400, "body must be a JSON object");
        }
        if (data == null || !data.isObject()) {
            throw new HttpError(400, "body must be a JSON object");
        }
        return data;
    }

    private static Input parseInput(HttpExchange ex) throws IOException, HttpError {
        JsonNode data = parseObject(ex);
        String url = requiredString(data, "url");
        String title = requiredString(data, "title");
        JsonNode desc = data.get("description");
        if (desc != null && !desc.isTextual()) {
            throw new HttpError(400, "description must be a string");
        }
        return new Input(url, title, desc == null ? "" : desc.asText());
    }

    private static String parseName(HttpExchange ex) throws IOException, HttpError {
        return requiredString(parseObject(ex), "name");
    }

    private static String requiredString(JsonNode data, String field) throws HttpError {
        JsonNode node = data.get(field);
        if (node == null || !node.isTextual() || node.asText().isBlank()) {
            throw new HttpError(400, field + " is required");
        }
        return node.asText().strip();
    }

    private static long requiredLong(JsonNode data, String field) throws HttpError {
        JsonNode node = data.get(field);
        if (node == null || node.isNull()) {
            throw new HttpError(400, field + " is required");
        }
        if (!node.isIntegralNumber() || !node.canConvertToLong()) {
            throw new HttpError(400, field + " must be an integer");
        }
        return node.asLong();
    }

    private static Long optionalLong(JsonNode data, String field) throws HttpError {
        JsonNode node = data.get(field);
        if (node == null || node.isNull()) {
            return null;
        }
        return requiredLong(data, field);
    }

    private static void writeJson(HttpExchange ex, int status, Object value) throws IOException {
        byte[] bytes = JSON.writeValueAsString(value).getBytes(StandardCharsets.UTF_8);
        ex.getResponseHeaders().set("Content-Type", "application/json");
        ex.sendResponseHeaders(status, bytes.length);
        try (OutputStream out = ex.getResponseBody()) {
            out.write(bytes);
        }
    }
}
