# Notes

## Assumptions:
- Right now the setupDatabase function first deletes the database if it exists and then creates a new one. This is not ideal for production. In production, we would want to check if the database exists and if it does not, then create a new one. If it does exist, we would want to update the schema if necessary.
- Currently it is all in a single file. In production, we would want to separate the code into different files for better organization and maintainability.