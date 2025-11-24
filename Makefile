# Generate documentation using gomarkdoc
README.md:
	@echo "Generating documentation..."
	gomarkdoc --output README.md ./...
