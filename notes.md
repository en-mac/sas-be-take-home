# Notes

## Thoughts/ Future Improvements:
- Right now a user_id is just a number related to its position in the database. The username could be used instead of user_id.

- All logs are currently using standard "log" package. In production, structured logging should be implemented so that we could easily search and filter logs like INFO, DEBUG, or ERROR. The logs could be streamed to a log aggregator like ELK stack or AWS CloudWatch.

- The setupDatabase function first deletes the database if it exists and then creates a new one. In production, there would likely be a database running persistently either in its own container or as a managed service. 

- Local caching via "go-cache" could be used to reduce time by eliminating duplicate API calls. In production, we could use Redis for caching.

- NLP could be used to ensure the responses only contain books in the language of the user.

