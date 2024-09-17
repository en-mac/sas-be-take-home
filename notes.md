# Notes

## Assumptions/ Future Improvements:
- Right now the setupDatabase function first deletes the database if it exists and then creates a new one. In production, we would likely have a database running persistently either in its own container or as a managed service.
- Right now a user_id is just a number related to its position in the database. We could do it using username instead of user_id.
- All logs are currently using standard "log" package. In production, we may want to use structured logging so that we could easily search and filter logs like INFO, DEBUG, or ERROR.
- We could add caching as needed using Redis to reduce the number of requests to the Open Library API. We might want to do this for the reccomended books since the published books for a given genre are unlikely to change frequently.
- The goroutines are set maximally to 10 Authors at once, 200 books at once, and 3 descriptions at once.