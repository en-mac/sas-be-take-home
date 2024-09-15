# Take Home Go Project: Read Together

### Make a REST API Function to Recommend Books for Two Users to Read Together

Your REST function will be given the IDs of two users. From that call you will be expected to return a JSON payload containing the recommended books for the two given IDs.

Get Two Lists of Authors, Up to Five Authors Listed Per Person from the Database
- You may presume the Author with the most works is the one being referred to if multiple authors w/ the same name exist.
- Of the authors the two users enjoy, find what genres, aka Subjects, are most prominent in the works of the authors. (Use a max of 100 books per author as a sample size.)

Find the subject most common amongst both lists of authors. (ex: List 1 has 3 authors that have written fantasy, and list 2 has 4 authors that have written fantasy.)
- Only recommend books still in print. (Defined as published in the last 2 years)
- Only fetch 50 books per subject.
- Recommend the three most recent books of that subject/genre providing the title, author and a description, if available, of each. 

### Relevant Links
REST API for Book & Author Info: [Open Library Swagger](https://openlibrary.org/swagger/docs#/)

API Documentation: [API Documentation](https://openlibrary.org/developers/api)

Dev Docs For Subject Querying: [https://openlibrary.org/dev/docs/api/subjects](https://openlibrary.org/dev/docs/api/subjects)


### Submission and Review
- Upon finishing your assignment, please be prepared to share your work on review day via a publicly accessible host. (GitHub, Pastebin, etc.)
- You will be expected to be able to walk through and execute your code as well as discuss it via screenshare.
- Questions may arise about extending the code in new ways, discussing new requirements, architecture, etc.