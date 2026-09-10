package cli

import nagicli "github.com/mayahiro/nagicli-go"

// CommandHelpDocuments returns the visible CLI help in command definition order.
// It validates the command graph without parsing arguments or running handlers.
func CommandHelpDocuments() ([]nagicli.HelpDocument, error) {
	var documents []nagicli.HelpDocument
	err := newNagiApplication().VisitHelpDocuments(func(document nagicli.HelpDocument) bool {
		documents = append(documents, document)
		return true
	})
	if err != nil {
		return nil, err
	}
	return documents, nil
}
