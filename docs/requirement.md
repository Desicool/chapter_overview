# Requirements
## User story
User will open the website, upload one huge pdf file, and the website will create a task immediately and redirect the user into the task page, where the user can see the progress of the task. After the task is done, the page will receive a notification and show the result to the user. The result will be the splited pdf chapters and the summary of each chapter. There will be at most 15 chapters no matter how big the original pdf file is. The user can click the chapter subtitle to jump into the pdf viewer page and see exactly the chapter content.

## System requirement
1. The system should be able to handle large pdf files.
2. The system should be able to split the pdf file into chapters and summarize each chapter.
3. The system should provide real-time progress updates to the user.
4. The system should be able to notify the user when the task is completed.
5. The system should summarize the chapter efficiently, ensuring that the summary is concise and informative. Using parallel processing to speed up the summarization of multiple chapters.
6. The system should split the pdf file precisely, ensuring that the chapters are correctly identified and separated. Using advanced algorithms to accurately identify chapter boundaries and split the pdf file accordingly.
7. The system should be able to handle various types of pdf files, including those with complex formatting and multiple languages. Implementing robust parsing techniques to ensure that the system can handle a wide range of pdf files without errors or issues.
8. The system should be scalable and able to handle multiple concurrent tasks without performance degradation. Implementing a distributed architecture to ensure that the system can handle a large number of tasks simultaneously without any performance issues.
9. The system should consider basic circuit breaker mechanism to prevent system overload and ensure stability. Implementing a circuit breaker pattern to monitor the system's performance and automatically stop accepting new tasks when the system is under heavy load, allowing it to recover and maintain stability.
