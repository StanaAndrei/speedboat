# Stage 1: Build stage
FROM alpine:3.20 AS builder

# Install build dependencies
RUN apk add --no-cache g++ make

WORKDIR /build

# Copy source file
COPY logic.cpp .

# Compile with high optimization and static linking for portability
RUN g++ -O3 -std=c++17 -static logic.cpp -o logic

# Stage 2: Minimal runtime container
FROM scratch

# Copy only the compiled static binary
COPY --from=builder /build/logic /app/logic

# Set entrypoint so passing stdin to the container executes the binary directly
ENTRYPOINT ["/app/logic"]