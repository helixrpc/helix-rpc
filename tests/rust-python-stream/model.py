class DummyModel:
    def __init__(self):
        print("Python: DummyModel initialized!")

    def generate_batch(self, prompts: list[str]) -> list[str]:
        print(f"Python: Received batch of {len(prompts)} prompts for inference.")
        responses = []
        for prompt in prompts:
            responses.append(f"AI response to: {prompt}")
        return responses

    def generate_stream(self, prompt: str):
        # A generator that yields chunks of the response
        words = ["This", " is", " a", " natively", " streamed", " response", " from", " Python", " via", " PyO3", "!"]
        for word in words:
            import time
            time.sleep(0.05) # Simulate inference latency
            yield word
