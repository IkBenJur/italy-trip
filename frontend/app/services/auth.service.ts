import { apiClient } from "~/lib/apiClient";
import type { AuthResponse, LoginInput } from "~/types/user.types";

// There is no register endpoint: the API serves one shared account, seeded from
// the environment on boot.
export const authService = {
  login(input: LoginInput): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>("/auth/login", input);
  },
};
