from fastapi import FastAPI

app = FastAPI()

@app.get("/")
def get_home_api():
    return {"status": "success"}
