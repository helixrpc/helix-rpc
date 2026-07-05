import uvicorn
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI()

class UserProfileResponse(BaseModel):
    user_id: int
    username: str
    email: str

@app.get("/v1/users/{user_id}", response_model=UserProfileResponse)
def get_user_profile(user_id: int):
    return {
        "user_id": user_id,
        "username": f"user-{user_id}-fastapi",
        "email": f"user-{user_id}@fastapi.com"
    }

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8001, log_level="warning")
