exception ComplexError {
    1: string message
}

union ComplexPayload {
    1: string text,
    2: binary data
}

struct ComplexRequest {
    1: i32 id,
    2: ComplexPayload payload,
    3: map<string, string> metadata
}
