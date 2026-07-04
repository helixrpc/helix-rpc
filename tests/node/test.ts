import { UserProfile } from './generated.js';

function assert(condition: boolean, message?: string) {
    if (!condition) {
        throw new Error(message || "Assertion failed");
    }
}

function runTest() {
    const original = new UserProfile({
        userId: 123456789,
        username: "node_flatbuffer_user",
        email: "node@test.com"
    });

    console.log("Marshaling original profile to FlatBuffers...");
    const buf = original.marshalFlatBuffers();
    assert(buf.length > 0, "Buffer should not be empty");

    console.log("Unmarshaling back...");
    const decoded = UserProfile.unmarshalFlatBuffers(buf);

    console.log("Asserting fields...");
    assert(decoded.userId === original.userId, `userId mismatch: got ${decoded.userId}, expected ${original.userId}`);
    assert(decoded.username === original.username, `username mismatch: got ${decoded.username}, expected ${original.username}`);
    assert(decoded.email === original.email, `email mismatch: got ${decoded.email}, expected ${original.email}`);

    console.log("✅ Node.js FlatBuffers codec test passed successfully!");
}

try {
    runTest();
} catch (err) {
    console.error("❌ Test failed:", err);
    process.exit(1);
}
