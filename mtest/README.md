How to manually run coil using placemat
======================================

1. Run `make setup`
2. Run `make placemat`
3. Login to `host1` by:

    ```console
    $ chmod 600 mtest_key
    $ ssh -i mtest_key cybozu@10.0.0.11
    ```

4. Run `/data/setup-cke.sh` on `host1`.
5. To stop placemat, run `make stop`.
