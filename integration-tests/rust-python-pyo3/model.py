class DummyModel:
    def __init__(self):
        print("Python: DummyModel initialized!")

    def generate_batch(self, prompts: list[str]) -> list[str]:
        print(f"Python: Received batch of {len(prompts)} prompts for inference.")
        responses = []
        for prompt in prompts:
            responses.append(f"AI response to: {prompt}")
        return responses
