// Package catalogaccess centralizes live catalog permissions for every data path.
package catalogaccess

// LibraryPredicate returns SQL with one user-ID placeholder. expression must be
// a trusted column expression from application code, never request input.
func LibraryPredicate(expression string) string {
	return libraryPredicateForUser(expression, "?")
}
func libraryPredicateForUser(expression, userExpression string) string {
	return `EXISTS (SELECT 1 FROM users acl_user WHERE acl_user.id = ` + userExpression + ` AND acl_user.disabled_at IS NULL AND
 (acl_user.role = 'admin' OR EXISTS (SELECT 1 FROM catalog_libraries acl_lib WHERE acl_lib.id = ` + expression + `
 AND (acl_lib.all_readers = 1 OR EXISTS (SELECT 1 FROM catalog_library_readers acl_reader
 WHERE acl_reader.library_id = acl_lib.id AND acl_reader.user_id = acl_user.id)))))`
}

// BookPredicate applies the same policy through the book's single library.
func BookPredicate(expression string) string {
	return BookPredicateForUser(expression, "?")
}

// BookPredicateForUser permits a trusted correlated user column instead of a placeholder.
func BookPredicateForUser(expression, userExpression string) string {
	return `EXISTS (SELECT 1 FROM catalog_library_books acl_book WHERE acl_book.book_id = ` + expression + ` AND ` + libraryPredicateForUser("acl_book.library_id", userExpression) + `)`
}
