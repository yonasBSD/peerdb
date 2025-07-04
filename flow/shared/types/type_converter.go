package types

//nolint:iface
type TypeConversion interface {
	SchemaConversion(QField) QField
	ValueConversion(QValue) QValue
	FromKind() QValueKind
}

type TypeConversionImpl[TFrom QValue, TTo QValue] struct {
	SchemaConversionFn func(QField) QField
	ValueConversionFn  func(TFrom) TTo
}

func (tc TypeConversionImpl[TFrom, TTo]) SchemaConversion(field QField) QField {
	return tc.SchemaConversionFn(field)
}

func (tc TypeConversionImpl[TFrom, TTo]) ValueConversion(val QValue) QValue {
	if _, ok := val.(QValueNull); ok {
		var toQ TTo
		return QValueNull(toQ.Kind())
	}
	return tc.ValueConversionFn(val.(TFrom))
}

func (tc TypeConversionImpl[TFrom, TTo]) FromKind() QValueKind {
	var fromQ TFrom
	return fromQ.Kind()
}

func NewTypeConversion[TFrom QValue, TTo QValue](
	schemaConversionFn func(QField) QField,
	valueConversionFn func(TFrom) TTo,
) TypeConversionImpl[TFrom, TTo] {
	return TypeConversionImpl[TFrom, TTo]{
		SchemaConversionFn: schemaConversionFn,
		ValueConversionFn:  valueConversionFn,
	}
}

func NumericToStringSchemaConversion(val QField) QField {
	val.Type = QValueKindString
	return val
}

func NumericToStringValueConversion(val QValueNumeric) QValueString {
	return QValueString{Val: val.Val.String()}
}

func NumericToInt256SchemaConversion(val QField) QField {
	val.Type = QValueKindInt256
	return val
}

func NumericToInt256ValueConversion(val QValueNumeric) QValueInt256 {
	return QValueInt256{Val: val.Val.BigInt()}
}

func NumericToUInt256SchemaConversion(val QField) QField {
	val.Type = QValueKindUInt256
	return val
}

func NumericToUInt256ValueConversion(val QValueNumeric) QValueUInt256 {
	return QValueUInt256{Val: val.Val.BigInt()}
}
