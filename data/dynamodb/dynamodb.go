package dynamodb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/haydenwoodhead/burner.kiwi/burner"
)

var _ burner.Database = &DynamoDB{}

// DynamoDB implements the db interface
type DynamoDB struct {
	dynDB                 *dynamodb.DynamoDB
	emailsTableName       string
	emailAddressIndexName string
}

//GetNewDynamoDB gets a new dynamodb database or panics
func GetNewDynamoDB(table string) *DynamoDB {
	awsSession := session.Must(session.NewSession())
	dynDB := dynamodb.New(awsSession)

	return &DynamoDB{
		dynDB:                 dynDB,
		emailsTableName:       table,
		emailAddressIndexName: "email_address-index",
	}
}

// SaveNewInbox saves a given inbox to dynamodb
func (d *DynamoDB) SaveNewInbox(i burner.Inbox) error {
	av, err := dynamodbattribute.MarshalMap(i)

	// Insert an empty messages attribute so we can add messages later
	av["messages"] = &dynamodb.AttributeValue{
		M: make(map[string]*dynamodb.AttributeValue),
	}

	if err != nil {
		return fmt.Errorf("SaveNewInbox: failed to marshal struct to attribute value: %v", err)

	}

	_, err = d.dynDB.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(d.emailsTableName),
		Item:      av,
	})

	if err != nil {
		return fmt.Errorf("SaveNewInbox: failed to put to dynamodb: %v", err)
	}

	return nil
}

//GetInboxByID gets an inbox by the given inbox id
func (d *DynamoDB) GetInboxByID(id string) (burner.Inbox, error) {
	var i burner.Inbox

	o, err := d.dynDB.GetItem(&dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(id),
			},
		},
		TableName: aws.String(d.emailsTableName),
	})

	if err != nil {
		return burner.Inbox{}, err
	}

	err = dynamodbattribute.UnmarshalMap(o.Item, &i)

	if err != nil {
		return burner.Inbox{}, err
	}

	return i, nil
}

type secondaryIndexInbox struct {
	ID           string `dynamodbav:"id"`
	EmailAddress string `dynamodbav:"email_address"`
}

func (d *DynamoDB) queryEmailIndex(address string) ([]secondaryIndexInbox, error) {
	q := &dynamodb.QueryInput{
		KeyConditionExpression: aws.String("email_address = :e"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":e": {
				S: aws.String(address),
			},
		},
		IndexName: aws.String(d.emailAddressIndexName),
		TableName: aws.String(d.emailsTableName),
	}

	res, err := d.dynDB.Query(q)
	if err != nil {
		return nil, err
	}

	var results []secondaryIndexInbox
	err = dynamodbattribute.UnmarshalListOfMaps(res.Items, &results)
	if err != nil {
		return nil, fmt.Errorf("queryEmailIndex: failed to unmarshal query")
	}

	return results, nil
}

func (d *DynamoDB) GetInboxByAddress(address string) (burner.Inbox, error) {
	res, err := d.queryEmailIndex(address)
	if err != nil {
		return burner.Inbox{}, fmt.Errorf("GetInboxByAddress: failed to get inbox: %v", err)
	}
	if len(res) == 0 {
		return burner.Inbox{}, errors.New("GetInboxByAddress: no inbox with address present")
	}

	return d.GetInboxByID(res[0].ID)
}

//EmailAddressExists returns a bool depending on whether or not the given email address
// is already assigned to an inbox
func (d *DynamoDB) EmailAddressExists(a string) (bool, error) {
	res, err := d.queryEmailIndex(a)
	if err != nil {
		return false, fmt.Errorf("EmailAddressExists: failed to query: %v", err)
	}

	return len(res) > 0, nil
}

// SetInboxCreated updates the given inbox to reflect its created status
func (d *DynamoDB) SetInboxCreated(i burner.Inbox) error {
	u := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#F": aws.String("failed_to_create"),
			"#M": aws.String("ep_routeid"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":f": {
				BOOL: aws.Bool(false),
			},
			":m": {
				S: aws.String(i.EmailProviderRouteID),
			},
		},
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(i.ID),
			},
		},
		TableName:        aws.String(d.emailsTableName),
		UpdateExpression: aws.String("SET #F = :f, #M = :m"),
	}

	_, err := d.dynDB.UpdateItem(u)

	if err != nil {
		return fmt.Errorf("SetInboxCreated: failed to mark email as created: %v", err)
	}

	return nil
}

// SetInboxFailed sets a given inbox as having failed to register with the mail provider
func (d *DynamoDB) SetInboxFailed(i burner.Inbox) error {
	u := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#F": aws.String("failed_to_create"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":f": {
				BOOL: aws.Bool(true),
			},
		},
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(i.ID),
			},
		},
		TableName:        aws.String(d.emailsTableName),
		UpdateExpression: aws.String("SET #F = :f"),
	}

	_, err := d.dynDB.UpdateItem(u)

	if err != nil {
		return fmt.Errorf("SetInboxFailed: failed to mark email as failed: %v", err)
	}

	return nil
}

//SaveNewMessage saves a given message to dynamodb
func (d *DynamoDB) SaveNewMessage(m burner.Message) error {
	mv, err := dynamodbattribute.MarshalMap(m)

	if err != nil {
		return fmt.Errorf("SaveMessage: failed to marshal struct to attribute value: %v", err)
	}

	_, err = d.dynDB.UpdateItem(&dynamodb.UpdateItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#M":   aws.String("messages"),
			"#MID": aws.String(m.ID),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":m": {
				M: mv,
			},
		},
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(m.InboxID),
			},
		},
		TableName:        aws.String(d.emailsTableName),
		UpdateExpression: aws.String("SET #M.#MID = :m"),
	})

	if err != nil {
		return fmt.Errorf("SaveMessage: failed to put to dynamodb: %v", err)
	}

	return nil
}

//GetMessagesByInboxID returns all messages in a given inbox
func (d *DynamoDB) GetMessagesByInboxID(i string) ([]burner.Message, error) {
	var ret map[string]map[string]burner.Message
	var msgs []burner.Message

	gi := &dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(i),
			},
		},
		ProjectionExpression: aws.String("messages"),
		TableName:            aws.String(d.emailsTableName),
	}

	res, err := d.dynDB.GetItem(gi)

	if err != nil {
		return []burner.Message{}, fmt.Errorf("GetAllMessagesByInboxID: failed to query dynamodb: %v", err)
	}

	err = dynamodbattribute.UnmarshalMap(res.Item, &ret)

	if err != nil {
		return []burner.Message{}, fmt.Errorf("GetAllMessagesByInboxID: failed to unmarshal result: %v", err)
	}

	for _, v := range ret["messages"] {
		msgs = append(msgs, v)
	}

	return msgs, nil
}

//GetMessageByID gets a single message by the given inbox and message id
func (d *DynamoDB) GetMessageByID(i, m string) (burner.Message, error) {
	var ret map[string]burner.Message

	gi := &dynamodb.GetItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#ID": aws.String(m),
		},
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(i),
			},
		},
		ProjectionExpression: aws.String("messages.#ID"),
		TableName:            aws.String(d.emailsTableName),
	}

	res, err := d.dynDB.GetItem(gi)

	if err != nil {
		return burner.Message{}, fmt.Errorf("GetMessageByID: failed to query dynamodb: %v", err)
	}

	// Despite only returning one message it is nested under messages and then it's id. We must unmarshal this response
	// to a map and then get the item from that map.
	err = dynamodbattribute.Unmarshal(res.Item["messages"], &ret)

	if err != nil {
		return burner.Message{}, fmt.Errorf("GetMessageByID: failed to unmarshal result: %v", err)
	}

	msg, ok := ret[m]

	if !ok {
		return burner.Message{}, burner.ErrMessageDoesntExist
	}

	return msg, nil
}

//createDatabase creates a new database for testing
func (d *DynamoDB) createDatabase() error {
	emails := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String("email_address"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("id"),
				KeyType:       aws.String("HASH"),
			},
		},
		GlobalSecondaryIndexes: []*dynamodb.GlobalSecondaryIndex{
			{
				IndexName: aws.String(d.emailAddressIndexName),
				KeySchema: []*dynamodb.KeySchemaElement{
					{
						AttributeName: aws.String("email_address"),
						KeyType:       aws.String("HASH"),
					},
				},
				Projection: &dynamodb.Projection{
					ProjectionType: aws.String(dynamodb.ProjectionTypeKeysOnly),
				},
				ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
		TableName: aws.String(d.emailsTableName),
	}

	_, err := d.dynDB.CreateTable(emails)

	if err != nil {
		if !strings.Contains(err.Error(), dynamodb.ErrCodeResourceInUseException) {
			return err
		}
	}

	return nil
}
