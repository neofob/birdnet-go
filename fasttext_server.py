from fastapi import FastAPI
from pydantic import BaseModel
import fasttext
app = FastAPI()
model = fasttext.load_model("lid.176.ftz")
class Request(BaseModel):
    text: str
class Prediction(BaseModel):
    label: str
    confidence: float
class Response(BaseModel):
    label: str
    confidence: float
    top_n: list[Prediction]
@app.post("/classify")
def classify(req: Request):
    labels, probs = model.predict(req.text, k=5)
    top_n = [
        Prediction(label=l.replace("__label__", ""), confidence=float(p))
        for l, p in zip(labels, probs)
    ]
    return Response(
        label=top_n[0].label,
        confidence=top_n[0].confidence,
        top_n=top_n,
    )