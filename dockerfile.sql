FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /src/sql .

CMD ["./sql"]