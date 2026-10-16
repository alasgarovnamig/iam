# OpenFGA

The authorization model is defined once, in the DSL file that the service
embeds and applies at startup:

    internal/openfga/model.fga

On boot the API:

1. looks up (or creates) the store named `wallet-gateway`,
2. transforms the DSL to a JSON authorization model,
3. writes the model if it differs from the latest one,
4. uses the resulting store/model ids for every `Check` and `Write`.

That makes the DSL the single source of truth — there is no hand-maintained
JSON copy to drift out of sync. Pin a specific store/model with the
`OPENFGA_STORE_ID` / `OPENFGA_MODEL_ID` environment variables when you need
deterministic deployments (recommended for production).

Inspect the model locally:

    docker run --rm -v "$PWD/internal/openfga:/m" \
      openfga/cli:latest model transform --file /m/model.fga
