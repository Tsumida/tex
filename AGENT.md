# Project overview

Tex(Tiny Exchange) consumers events from kafka topics and transfer them into K-Bar, Account Snapshot and OrderList in Redis.
Tex also serves query form Web portal.

# Build and test commands

# Go style guidelines
- No panic while app serving. For unimplemented code, use panic("todo") and you should always do it.
- Prefer seperate read \ write in the same code block, try no to update a container while iterating it.
- Prevent mix mutex and business code in the same code block. Using wrapper like LockAndDo(func()).
- Always add "Write" comment if function modify any of its arguments.
- Always return (T, Error) while conver a ProtoBuf object into other type since external data could be invalid.
- Always return (T, Error) if function deals with anything external like OS, RPC or FileSystem.
- Prefer dependency injection for simplifying mocking. 
- Prefer implemention first. 
- Always when writing tests, prefer comparing the equality of entire objects over fields one by one.
- Always use `os.Getenv("XXX")` instead of alias, so we can use `make list-env` to list all env required. 
  
# Go pkg guidelines
- Always use `https://github.com/IBM/sarama` as kafka client to avoid complex 

# Testing instructions
- Run `make ut`.

# Security considerations
