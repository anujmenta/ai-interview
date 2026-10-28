package bookmarks;

public record Bookmark(long id, String url, String title, String description, String createdAt) {}
