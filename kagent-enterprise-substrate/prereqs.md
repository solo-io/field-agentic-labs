## Kagent v0 vs v1

If you're on kagent v0, there is not a straightforward upgrade. You need need to manually migrate the workloads. It's a matter of "take what you have deployed today and update the YAML into kagent v1 objects". There needs to be a clean install of kagent, then create the CRDs in there. There is no data/database migration at all - you need a fresh database for this.