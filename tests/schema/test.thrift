namespace go helix.example

struct UserProfile {
	1: i64 user_id;
	2: string username;
	3: string email;
}

service UserProfileService {
	UserProfile GetUserProfile(1: UserProfile request)
}
