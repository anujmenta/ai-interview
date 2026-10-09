from flask import Flask

from db import init_db
from routes import bp

app = Flask(__name__)
app.register_blueprint(bp)

init_db()

if __name__ == "__main__":
    app.run(port=5000, debug=True)
