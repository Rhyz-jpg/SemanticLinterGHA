# Semantic Linting Rules

## Naming Conventions
- Variable and function names must be descriptive and follow camelCase
- Class names must use PascalCase
- Constants should be UPPER_SNAKE_CASE
- Private members should be prefixed with underscore
- Boolean variables should start with is/has/should

## Code Complexity
- Functions should not exceed 20 lines
- Maximum nesting depth of 3 levels
- Avoid multiple return statements
- Keep cyclomatic complexity under 10
- Maximum of 3 parameters for functions

## Documentation
- All public APIs must have JSDoc comments
- Complex algorithms need explanatory comments
- TODO comments must include ticket numbers
- Update comments when changing code
- Document non-obvious side effects

## Best Practices
- Prefer early returns
- Avoid magic numbers
- Use meaningful variable names
- Keep functions single-purpose
- Use TypeScript types explicitly
- Handle all possible error cases
- Avoid global state

## Error Handling
- Use try-catch blocks appropriately
- Avoid swallowing errors
- Log errors with context
- Return meaningful error messages

## Code Style
- Use consistent indentation
- Add spaces around operators
- One statement per line
- Maximum line length of 80 characters
- Use semicolons consistently
