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

    private record CollectionInput(String name) {}

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

    // dispatches every /collections[...] route by splitting the path into
    // segments, since this handler has no path-variable router of its own
    private void routeCollections(HttpExchange ex, String method, String path)
            throws IOException, SQLException, HttpError {
        String[] parts = path.split("/");
        // parts[0] is always "" (leading slash); parts[1] is "collections"

        if (parts.length == 2) {
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.listCollections());
                case "POST" -> {
                    CollectionInput in = parseCollectionInput(ex);
                    writeJson(ex, 201, store.createCollection(in.name()));
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        long collectionId = parseId(parts[2], COLLECTION_NOT_FOUND);

        if (parts.length == 3) {
            switch (method) {
                case "GET" -> writeJson(ex, 200, store.getCollection(collectionId)
                    .orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                case "PUT" -> {
                    CollectionInput in = parseCollectionInput(ex);
                    Optional<Collection> updated = store.updateCollection(collectionId, in.name());
                    writeJson(ex, 200, updated.orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND)));
                }
                case "DELETE" -> {
                    if (!store.deleteCollection(collectionId)) {
                        throw new HttpError(404, COLLECTION_NOT_FOUND);
                    }
                    ex.sendResponseHeaders(204, -1);
                }
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        if (parts.length < 4 || !parts[3].equals("bookmarks")) {
            throw new HttpError(404, "not found");
        }

        if (parts.length == 4) {
            switch (method) {
                case "GET" -> {
                    store.getCollection(collectionId).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND));
                    writeJson(ex, 200, store.listCollectionBookmarks(collectionId));
                }
                case "POST" -> writeJson(ex, 201, addBookmarkToCollection(ex, collectionId));
                default -> throw new HttpError(405, "method not allowed");
            }
            return;
        }

        long bookmarkId = parseId(parts[4]);

        if (parts.length == 5) {
            if (!method.equals("DELETE")) {
                throw new HttpError(405, "method not allowed");
            }
            store.getCollection(collectionId).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND));
            if (!store.removeBookmarkFromCollection(collectionId, bookmarkId)) {
                throw new HttpError(404, NOT_IN_COLLECTION);
            }
            ex.sendResponseHeaders(204, -1);
            return;
        }

        if (parts.length == 6 && parts[5].equals("position")) {
            if (!method.equals("PUT")) {
                throw new HttpError(405, "method not allowed");
            }
            store.getCollection(collectionId).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND));
            int position = parsePosition(ex);
            MoveResult result = store.moveBookmark(collectionId, bookmarkId, position);
            switch (result.status()) {
                case NOT_MEMBER -> throw new HttpError(404, NOT_IN_COLLECTION);
                case OUT_OF_RANGE -> throw new HttpError(400, "position is out of range");
                case OK -> writeJson(ex, 200, result.bookmarks());
            }
            return;
        }

        throw new HttpError(404, "not found");
    }

    private CollectionBookmark addBookmarkToCollection(HttpExchange ex, long collectionId)
            throws IOException, SQLException, HttpError {
        store.getCollection(collectionId).orElseThrow(() -> new HttpError(404, COLLECTION_NOT_FOUND));
        long bookmarkId = parseBookmarkId(ex);
        store.getBookmark(bookmarkId).orElseThrow(() -> new HttpError(404, NOT_FOUND));
        return store.addBookmarkToCollection(collectionId, bookmarkId)
            .orElseThrow(() -> new HttpError(409, "bookmark already in collection"));
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

    private static JsonNode readJsonObject(HttpExchange ex) throws IOException, HttpError {
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
        JsonNode data = readJsonObject(ex);
        String url = requiredString(data, "url");
        String title = requiredString(data, "title");
        JsonNode desc = data.get("description");
        if (desc != null && !desc.isTextual()) {
            throw new HttpError(400, "description must be a string");
        }
        return new Input(url, title, desc == null ? "" : desc.asText());
    }

    private static CollectionInput parseCollectionInput(HttpExchange ex) throws IOException, HttpError {
        return new CollectionInput(requiredString(readJsonObject(ex), "name"));
    }

    private static long parseBookmarkId(HttpExchange ex) throws IOException, HttpError {
        return requiredLong(readJsonObject(ex), "bookmark_id");
    }

    private static int parsePosition(HttpExchange ex) throws IOException, HttpError {
        long value = requiredLong(readJsonObject(ex), "position");
        if (value < Integer.MIN_VALUE || value > Integer.MAX_VALUE) {
            throw new HttpError(400, "position is out of range");
        }
        return (int) value;
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
        if (node == null || !node.isIntegralNumber()) {
            throw new HttpError(400, field + " is required");
        }
        return node.asLong();
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
