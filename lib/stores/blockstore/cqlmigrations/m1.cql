-- block table

CREATE TABLE blocks (
    hash blob,
    number bigint,
    chain ascii,

    flags tinyint,

    header varchar,
    receipts varchar,

    primary key (chain, number)
);

CREATE INDEX blocksByHash on blocks(hash);
