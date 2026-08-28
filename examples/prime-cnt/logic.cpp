#include <iostream>
#include <string>
#include <sstream>
#include <cmath>

// Minimal self-contained JSON extraction to avoid external C++ deps in templates
long long extract_int(const std::string& json, const std::string& key) {
    size_t pos = json.find("\"" + key + "\"");
    if (pos == std::string::npos) return 0;
    size_t colon = json.find(":", pos);
    size_t start = json.find_first_of("0123456789", colon);
    size_t end = json.find_first_not_of("0123456789", start);
    return std::stoll(json.substr(start, end - start));
}

std::string extract_string(const std::string& json, const std::string& key) {
    size_t pos = json.find("\"" + key + "\"");
    if (pos == std::string::npos) return "";
    size_t first_quote = json.find("\"", pos + key.length() + 2);
    size_t second_quote = json.find("\"", first_quote + 1);
    return json.substr(first_quote + 1, second_quote - first_quote - 1);
}

bool is_prime(long long n) {
    if (n <= 1) return false;
    if (n <= 3) return true;
    if (n % 2 == 0 || n % 3 == 0) return false;
    for (long long i = 5; i * i <= n; i += 6) {
        if (n % i == 0 || n % (i + 2) == 0) return false;
    }
    return true;
}

int main() {
    // 1. Read task JSON from stdin
    std::string input_buffer;
    std::string line;
    while (std::getline(std::cin, line)) {
        input_buffer += line;
    }

    if (input_buffer.empty()) {
        std::cerr << "Error: No task data received on stdin\n";
        return 1;
    }

    std::string task_id = extract_string(input_buffer, "task_id");
    long long start = extract_int(input_buffer, "start");
    long long end = extract_int(input_buffer, "end");

    // 2. Perform compute
    long long prime_count = 0;
    for (long long i = start; i <= end; ++i) {
        if (is_prime(i)) {
            prime_count++;
        }
    }

    // 3. Emit Result JSON to stdout[cite: 1]
    std::cout << "{\n"
              << "  \"task_id\": \"" << task_id << "\",\n"
              << "  \"status\": \"ok\",\n"
              << "  \"output\": {\n"
              << "    \"start\": " << start << ",\n"
              << "    \"end\": " << end << ",\n"
              << "    \"count\": " << prime_count << "\n"
              << "  }\n"
              << "}\n";

    return 0;
}