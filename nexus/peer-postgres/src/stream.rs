use std::{
    pin::Pin,
    task::{Context, Poll},
};

use bytes::Bytes;
use chrono::{DateTime, NaiveDate, NaiveDateTime, NaiveTime, Utc};
use futures::Stream;
use peer_cursor::{Record, RecordStream, SchemaRef};
use pgerror::PgError;
use pgwire::error::{PgWireError, PgWireResult};
use tokio_postgres::{types::Type, Row, RowStream};
use uuid::Uuid;
use value::{array::ArrayValue, Value};

pub struct PgRecordStream {
    row_stream: Pin<Box<RowStream>>,
    schema: SchemaRef,
}

impl PgRecordStream {
    pub fn new(row_stream: RowStream, schema: SchemaRef) -> Self {
        Self {
            row_stream: Box::pin(row_stream),
            schema,
        }
    }
}

fn values_from_row(row: &Row) -> Vec<Value> {
    (0..row.len())
        .map(|i| {
            let col_type = row.columns()[i].type_();
            match col_type {
                &Type::BOOL => row
                    .get::<_, Option<bool>>(i)
                    .map(Value::Bool)
                    .unwrap_or(Value::Null),
                &Type::CHAR => {
                    let ch: Option<i8> = row.get(i);
                    ch.map(|c| char::from_u32(c as u32).unwrap_or('\0'))
                        .map(Value::Char)
                        .unwrap_or(Value::Null)
                }
                &Type::VARCHAR | &Type::TEXT | &Type::BPCHAR => {
                    let s: Option<String> = row.get(i);
                    s.map(Value::Text).unwrap_or(Value::Null)
                }
                &Type::VARCHAR_ARRAY | &Type::BPCHAR_ARRAY => {
                    let s: Option<Vec<String>> = row.get(i);
                    s.map(ArrayValue::VarChar)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::NAME
                | &Type::NAME_ARRAY
                | &Type::REGPROC
                | &Type::REGPROCEDURE
                | &Type::REGOPER
                | &Type::REGOPERATOR
                | &Type::REGCLASS
                | &Type::REGTYPE
                | &Type::REGCONFIG
                | &Type::REGDICTIONARY
                | &Type::REGNAMESPACE
                | &Type::REGROLE
                | &Type::REGCOLLATION
                | &Type::REGPROCEDURE_ARRAY
                | &Type::REGOPER_ARRAY
                | &Type::REGOPERATOR_ARRAY
                | &Type::REGCLASS_ARRAY
                | &Type::REGTYPE_ARRAY
                | &Type::REGCONFIG_ARRAY
                | &Type::REGDICTIONARY_ARRAY
                | &Type::REGNAMESPACE_ARRAY
                | &Type::REGROLE_ARRAY
                | &Type::REGCOLLATION_ARRAY => {
                    let s: Option<String> = row.get(i);
                    s.map(Value::Text).unwrap_or(Value::Null)
                }
                &Type::INT2 => {
                    let int: Option<i16> = row.get(i);
                    int.map(Value::SmallInt).unwrap_or(Value::Null)
                }
                &Type::INT2_ARRAY => {
                    let int: Option<Vec<i16>> = row.get(i);
                    int.map(ArrayValue::SmallInt)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::INT4
                | &Type::TID
                | &Type::XID
                | &Type::CID
                | &Type::PG_NDISTINCT
                | &Type::PG_DEPENDENCIES => {
                    let int: Option<i32> = row.get(i);
                    int.map(Value::Integer).unwrap_or(Value::Null)
                }
                &Type::INT4_ARRAY
                | &Type::TID_ARRAY
                | &Type::XID_ARRAY
                | &Type::CID_ARRAY
                | &Type::OID_VECTOR
                | &Type::OID_VECTOR_ARRAY => {
                    let int: Option<Vec<i32>> = row.get(i);
                    int.map(ArrayValue::Integer)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::INT8 => {
                    let big_int: Option<i64> = row.get(i);
                    big_int.map(Value::BigInt).unwrap_or(Value::Null)
                }
                &Type::INT8_ARRAY => {
                    let big_int: Option<Vec<i64>> = row.get(i);
                    big_int
                        .map(ArrayValue::BigInt)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::OID => {
                    let oid: Option<u32> = row.get(i);
                    oid.map(Value::Oid).unwrap_or(Value::Null)
                }
                &Type::FLOAT4 => {
                    let float: Option<f32> = row.get(i);
                    float.map(Value::Float).unwrap_or(Value::Null)
                }
                &Type::FLOAT4_ARRAY => {
                    let float: Option<Vec<f32>> = row.get(i);
                    float
                        .map(ArrayValue::Float)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::FLOAT8 => {
                    let float: Option<f64> = row.get(i);
                    float.map(Value::Double).unwrap_or(Value::Null)
                }
                &Type::FLOAT8_ARRAY => {
                    let float: Option<Vec<f64>> = row.get(i);
                    float
                        .map(ArrayValue::Double)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::NUMERIC => {
                    let numeric: Option<String> = row.get(i);
                    numeric.map(Value::Numeric).unwrap_or(Value::Null)
                }
                &Type::NUMERIC_ARRAY => {
                    let numeric: Option<Vec<String>> = row.get(i);
                    numeric
                        .map(ArrayValue::Numeric)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::BYTEA => {
                    let bytes: Option<&[u8]> = row.get(i);
                    let bytes = bytes.map(Bytes::copy_from_slice);
                    bytes.map(Value::VarBinary).unwrap_or(Value::Null)
                }
                &Type::BYTEA_ARRAY => {
                    let bytes: Option<Vec<&[u8]>> = row.get(i);
                    let bytes = bytes.map(|bytes| {
                        bytes
                            .iter()
                            .map(|bytes| Bytes::copy_from_slice(bytes))
                            .collect()
                    });
                    bytes
                        .map(ArrayValue::VarBinary)
                        .map(Value::Array)
                        .unwrap_or(Value::Null)
                }
                &Type::JSON | &Type::JSONB => {
                    let jsonb: Option<serde_json::Value> = row.get(i);
                    jsonb.map(Value::JsonB).unwrap_or(Value::Null)
                }
                &Type::UUID => {
                    let uuid: Option<Uuid> = row.get(i);
                    uuid.map(Value::Uuid).unwrap_or(Value::Null)
                }
                &Type::INET | &Type::CIDR => {
                    let s: Option<String> = row.get(i);
                    s.map(Value::Text).unwrap_or(Value::Null)
                }
                &Type::POINT
                | &Type::POINT_ARRAY
                | &Type::LINE
                | &Type::LINE_ARRAY
                | &Type::LSEG
                | &Type::LSEG_ARRAY
                | &Type::BOX
                | &Type::BOX_ARRAY
                | &Type::POLYGON
                | &Type::POLYGON_ARRAY
                | &Type::CIRCLE
                | &Type::CIRCLE_ARRAY => Value::Text(row.get(i)),

                &Type::TIMESTAMP => {
                    let dt_utc: Option<NaiveDateTime> = row.get(i);
                    dt_utc.map(Value::postgres_timestamp).unwrap_or(Value::Null)
                }
                &Type::TIMESTAMPTZ => {
                    let dt_utc: Option<DateTime<Utc>> = row.get(i);
                    dt_utc
                        .map(Value::TimestampWithTimeZone)
                        .unwrap_or(Value::Null)
                }
                &Type::DATE => {
                    let t: Option<NaiveDate> = row.get(i);
                    t.map(Value::Date).unwrap_or(Value::Null)
                }
                &Type::TIME => {
                    let t: Option<NaiveTime> = row.get(i);
                    t.map(Value::Time).unwrap_or(Value::Null)
                }
                &Type::TIMETZ => {
                    let t: Option<NaiveTime> = row.get(i);
                    t.map(Value::TimeWithTimeZone).unwrap_or(Value::Null)
                }
                &Type::INTERVAL => {
                    let iv: Option<String> = row.get(i);
                    iv.map(Value::Text).unwrap_or(Value::Null)
                }
                &Type::ANY => Value::Text(row.get(i)),
                &Type::ANYARRAY => {
                    todo!("Array type conversion not implemented yet")
                }
                &Type::VOID => Value::Null,
                &Type::TRIGGER => Value::Text(row.get(i)),
                &Type::LANGUAGE_HANDLER => Value::Text(row.get(i)),
                &Type::INTERNAL => Value::Null,
                &Type::ANYELEMENT => Value::Text(row.get(i)),
                &Type::ANYNONARRAY
                | &Type::ANYCOMPATIBLE
                | &Type::ANYCOMPATIBLEARRAY
                | &Type::ANYCOMPATIBLENONARRAY
                | &Type::ANYCOMPATIBLEMULTI_RANGE
                | &Type::ANYMULTI_RANGE => Value::Text(row.get(i)),
                &Type::TXID_SNAPSHOT | &Type::TXID_SNAPSHOT_ARRAY => Value::Text(row.get(i)),
                &Type::FDW_HANDLER => Value::Text(row.get(i)),
                &Type::PG_LSN | &Type::PG_LSN_ARRAY => Value::Text(row.get(i)),
                &Type::PG_SNAPSHOT | &Type::PG_SNAPSHOT_ARRAY => Value::Text(row.get(i)),
                &Type::XID8 | &Type::XID8_ARRAY => Value::Text(row.get(i)),
                &Type::TS_VECTOR | &Type::TS_VECTOR_ARRAY => Value::Text(row.get(i)),
                &Type::TSQUERY | &Type::TSQUERY_ARRAY => Value::Text(row.get(i)),
                &Type::NUMMULTI_RANGE
                | &Type::NUMMULTI_RANGE_ARRAY
                | &Type::TSMULTI_RANGE
                | &Type::TSMULTI_RANGE_ARRAY
                | &Type::TSTZMULTI_RANGE
                | &Type::TSTZMULTI_RANGE_ARRAY
                | &Type::DATEMULTI_RANGE
                | &Type::DATEMULTI_RANGE_ARRAY
                | &Type::INT4MULTI_RANGE
                | &Type::INT4MULTI_RANGE_ARRAY
                | &Type::INT8MULTI_RANGE
                | &Type::INT8MULTI_RANGE_ARRAY => Value::Text(row.get(i)),
                _ => panic!("unsupported col type: {:?}", col_type),
            }
        })
        .collect()
}

impl Stream for PgRecordStream {
    type Item = PgWireResult<Record>;

    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        let schema = self.schema.clone();
        let row_stream = &mut self.row_stream;

        match Pin::new(row_stream).poll_next(cx) {
            Poll::Ready(Some(Ok(row))) => {
                let values = values_from_row(&row);
                let record = Record { values, schema };
                Poll::Ready(Some(Ok(record)))
            }
            Poll::Ready(Some(Err(e))) => {
                let err = Box::new(PgError::Internal {
                    err_msg: e.to_string(),
                });
                let err = PgWireError::ApiError(err);
                Poll::Ready(Some(Err(err)))
            }
            Poll::Ready(None) => Poll::Ready(None),
            Poll::Pending => Poll::Pending,
        }
    }
}

impl RecordStream for PgRecordStream {
    fn schema(&self) -> SchemaRef {
        self.schema.clone()
    }
}