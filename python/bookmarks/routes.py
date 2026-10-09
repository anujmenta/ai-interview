from flask import Blueprint, jsonify, request

import models

bp = Blueprint("bookmarks", __name__)


def validate_bookmark(data):
    if not isinstance(data, dict):
        return "body must be a JSON object"
    for field in ("url", "title"):
        if not isinstance(data.get(field), str) or not data[field].strip():
            return f"{field} is required"
    if not isinstance(data.get("description", ""), str):
        return "description must be a string"
    return None


@bp.get("/bookmarks")
def list_bookmarks():
    return jsonify(models.list_bookmarks())


@bp.get("/bookmarks/<int:bookmark_id>")
def get_bookmark(bookmark_id):
    bookmark = models.get_bookmark(bookmark_id)
    if bookmark is None:
        return jsonify(error="bookmark not found"), 404
    return jsonify(bookmark)


@bp.post("/bookmarks")
def create_bookmark():
    data = request.get_json(silent=True)
    error = validate_bookmark(data)
    if error:
        return jsonify(error=error), 400
    bookmark = models.create_bookmark(
        data["url"].strip(), data["title"].strip(), data.get("description", "")
    )
    return jsonify(bookmark), 201


@bp.put("/bookmarks/<int:bookmark_id>")
def update_bookmark(bookmark_id):
    data = request.get_json(silent=True)
    error = validate_bookmark(data)
    if error:
        return jsonify(error=error), 400
    bookmark = models.update_bookmark(
        bookmark_id, data["url"].strip(), data["title"].strip(), data.get("description", "")
    )
    if bookmark is None:
        return jsonify(error="bookmark not found"), 404
    return jsonify(bookmark)


@bp.delete("/bookmarks/<int:bookmark_id>")
def delete_bookmark(bookmark_id):
    if not models.delete_bookmark(bookmark_id):
        return jsonify(error="bookmark not found"), 404
    return "", 204
