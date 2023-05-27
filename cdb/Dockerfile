FROM cockroachdb/cockroach

COPY cdb/migrations /migrations
COPY cdb/bootstrap-db.sh .

ENTRYPOINT ["bash", "./bootstrap-db.sh"]
