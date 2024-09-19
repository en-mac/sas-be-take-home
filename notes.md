# Notes

## Assumptions/ Future Improvements:
- Right now a user_id is just a number related to its position in the database. We could do it using username instead of user_id.
- All logs are currently using standard "log" package and are streamed to logs.db. In production, we may want to use structured logging so that we could easily search and filter logs like INFO, DEBUG, or ERROR. We could stream these logs to a log aggregator like ELK stack or AWS CloudWatch.
- Right now the setupDatabase function first deletes the database if it exists and then creates a new one. In production, we would likely have a database running persistently either in its own container or as a managed service. 

- I need to use local caching to reduce time by eliminating duplicate API calls. I will be using "go-cache" for this purpose. In production, we could use Redis for caching.
- Then we can see if we need to use machine learning to find a more accurate 'best recommendation' for the users.


# TODO:
- stream logs to a new logs.db file- implement local caching
- machine learning over subjects
